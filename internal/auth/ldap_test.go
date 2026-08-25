package auth

import "testing"

func TestBuildGroupFilterDefaultTemplate(t *testing.T) {
	a := NewLDAPAuthenticator(Config{}, nil, nil)
	filter, err := a.buildGroupFilter("cn=alice,ou=users,dc=example,dc=com", "alice")
	if err != nil {
		t.Fatalf("buildGroupFilter: %v", err)
	}
	want := "(&(objectClass=groupOfNames)(member=cn=alice,ou=users,dc=example,dc=com))"
	if filter != want {
		t.Errorf("filter = %q, want %q", filter, want)
	}
}

func TestBuildGroupFilterEscapesUserInput(t *testing.T) {
	a := NewLDAPAuthenticator(Config{}, nil, nil)
	// A DN/uid containing LDAP filter metacharacters must come out escaped,
	// never interpolated raw into the filter string.
	filter, err := a.buildGroupFilter("cn=a)(uid=*,dc=example,dc=com", "alice")
	if err != nil {
		t.Fatalf("buildGroupFilter: %v", err)
	}
	want := "(&(objectClass=groupOfNames)(member=cn=a\\29\\28uid=\\2a,dc=example,dc=com))"
	if filter != want {
		t.Errorf("filter = %q, want %q", filter, want)
	}
}

func TestBuildGroupFilterCustomTemplateUsesUID(t *testing.T) {
	tmpl, err := ParseGroupFilterTemplate("(&(objectClass=posixGroup)(memberUid={{.UID}}))")
	if err != nil {
		t.Fatalf("ParseGroupFilterTemplate: %v", err)
	}
	a := NewLDAPAuthenticator(Config{}, nil, tmpl)
	filter, err := a.buildGroupFilter("cn=alice,ou=users,dc=example,dc=com", "alice")
	if err != nil {
		t.Fatalf("buildGroupFilter: %v", err)
	}
	want := "(&(objectClass=posixGroup)(memberUid=alice))"
	if filter != want {
		t.Errorf("filter = %q, want %q", filter, want)
	}
}
