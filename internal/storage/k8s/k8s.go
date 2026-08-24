// Package k8s implements storage.VaultStorageEngine over the Kubernetes
// Secrets API — used for Cluster Mode, where AccessLens runs inside the
// cluster it manages secrets for.
//
// Vault paths map onto Kubernetes Secrets as "secretName" (a directory of
// keys) or "secretName/key" (a single value).
package k8s

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/accesslens/accesslens/internal/model"
	"github.com/accesslens/accesslens/internal/storage"
)

const requestTimeout = 10 * time.Second

// Engine implements storage.VaultStorageEngine over Kubernetes Secrets in a
// single namespace.
type Engine struct {
	client    kubernetes.Interface
	namespace string
	audit     storage.AuditRecorder
}

var _ storage.VaultStorageEngine = (*Engine)(nil)

// New creates an Engine from an existing Kubernetes client, e.g. one built
// from a fake clientset in tests.
func New(client kubernetes.Interface, namespace string, audit storage.AuditRecorder) *Engine {
	return &Engine{client: client, namespace: namespace, audit: audit}
}

// NewInCluster builds an Engine using the in-cluster service account
// config, for when AccessLens is deployed inside the cluster it serves.
func NewInCluster(namespace string, audit storage.AuditRecorder) (*Engine, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("load in-cluster config: %w", err)
	}
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("create kubernetes client: %w", err)
	}
	return New(clientset, namespace, audit), nil
}

// splitPath separates a logical vault path into a Secret name and,
// optionally, a data key within it.
func splitPath(path string) (secretName, key string) {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return "", ""
	}
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return parts[0], ""
}

func (e *Engine) List(path string) ([]model.FileItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	secretName, key := splitPath(path)

	if secretName == "" {
		secrets, err := e.client.CoreV1().Secrets(e.namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, fmt.Errorf("list secrets: %w", err)
		}
		items := make([]model.FileItem, 0, len(secrets.Items))
		for _, s := range secrets.Items {
			items = append(items, model.FileItem{Path: s.Name, Name: s.Name, IsDir: true})
		}
		return items, nil
	}
	if key != "" {
		return nil, fmt.Errorf("path %q references a key, not a secret", path)
	}

	secret, err := e.client.CoreV1().Secrets(e.namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get secret %q: %w", secretName, err)
	}
	items := make([]model.FileItem, 0, len(secret.Data))
	for k := range secret.Data {
		items = append(items, model.FileItem{Path: secretName + "/" + k, Name: k, IsDir: false})
	}
	return items, nil
}

func (e *Engine) Read(path string) (*model.VaultFile, error) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	secretName, key := splitPath(path)
	if key == "" {
		return nil, fmt.Errorf("path %q must reference a key within a secret", path)
	}

	secret, err := e.client.CoreV1().Secrets(e.namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get secret %q: %w", secretName, err)
	}
	content, ok := secret.Data[key]
	if !ok {
		return nil, fmt.Errorf("key %q not found in secret %q", key, secretName)
	}
	return &model.VaultFile{Path: path, Content: content}, nil
}

func (e *Engine) Save(path string, content []byte, user, reason string) error {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	secretName, key := splitPath(path)
	if key == "" {
		return fmt.Errorf("path %q must reference a key within a secret", path)
	}

	secrets := e.client.CoreV1().Secrets(e.namespace)
	secret, err := secrets.Get(ctx, secretName, metav1.GetOptions{})
	notFound := apierrors.IsNotFound(err)
	if err != nil && !notFound {
		return fmt.Errorf("get secret %q: %w", secretName, err)
	}

	action := "update"
	if notFound {
		action = "create"
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: e.namespace},
			Type:       corev1.SecretTypeOpaque,
			Data:       map[string][]byte{},
		}
	} else if _, exists := secret.Data[key]; !exists {
		action = "create"
	}
	if secret.Data == nil {
		secret.Data = map[string][]byte{}
	}
	secret.Data[key] = content

	if notFound {
		if _, err := secrets.Create(ctx, secret, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("create secret %q: %w", secretName, err)
		}
	} else if _, err := secrets.Update(ctx, secret, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update secret %q: %w", secretName, err)
	}

	return e.audit.Record(model.AuditLog{
		Path:      path,
		Action:    action,
		User:      user,
		Reason:    reason,
		Timestamp: time.Now(),
	})
}

func (e *Engine) Delete(path string, user string) error {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	secretName, key := splitPath(path)
	secrets := e.client.CoreV1().Secrets(e.namespace)

	if key == "" {
		if err := secrets.Delete(ctx, secretName, metav1.DeleteOptions{}); err != nil {
			return fmt.Errorf("delete secret %q: %w", secretName, err)
		}
	} else {
		secret, err := secrets.Get(ctx, secretName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("get secret %q: %w", secretName, err)
		}
		if _, exists := secret.Data[key]; !exists {
			return fmt.Errorf("key %q not found in secret %q", key, secretName)
		}
		delete(secret.Data, key)
		if _, err := secrets.Update(ctx, secret, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update secret %q: %w", secretName, err)
		}
	}

	return e.audit.Record(model.AuditLog{
		Path:      path,
		Action:    "delete",
		User:      user,
		Timestamp: time.Now(),
	})
}

func (e *Engine) Rename(oldPath, newPath, user, reason string) error {
	oldSecret, oldKey := splitPath(oldPath)
	newSecret, newKey := splitPath(newPath)
	if oldKey == "" || newKey == "" {
		return fmt.Errorf("rename requires paths that reference a key within a secret")
	}
	if oldSecret != newSecret {
		return fmt.Errorf("rename must stay within the same secret (namespace 이동은 지원하지 않음): %q -> %q", oldPath, newPath)
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	secrets := e.client.CoreV1().Secrets(e.namespace)
	secret, err := secrets.Get(ctx, oldSecret, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get secret %q: %w", oldSecret, err)
	}
	value, exists := secret.Data[oldKey]
	if !exists {
		return fmt.Errorf("key %q not found in secret %q", oldKey, oldSecret)
	}
	if _, exists := secret.Data[newKey]; exists {
		return fmt.Errorf("key %q already exists in secret %q", newKey, oldSecret)
	}
	secret.Data[newKey] = value
	delete(secret.Data, oldKey)
	if _, err := secrets.Update(ctx, secret, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update secret %q: %w", oldSecret, err)
	}

	return e.audit.Record(model.AuditLog{
		Path:         newPath,
		PreviousPath: oldPath,
		Action:       "rename",
		User:         user,
		Reason:       reason,
		Timestamp:    time.Now(),
	})
}

func (e *Engine) GetHistory(path string) ([]model.AuditLog, error) {
	return e.audit.History(path)
}

// CreateNamespace is not supported: a Kubernetes Secret namespace has no
// empty state to create — it exists once it holds at least one Secret.
func (e *Engine) CreateNamespace(path string, user string) error {
	return fmt.Errorf("cluster mode has no empty-namespace concept: create a secret key under %q instead", path)
}

func (e *Engine) Search(query string) ([]model.SearchResult, error) {
	return storage.WalkAndSearch(e, query)
}
