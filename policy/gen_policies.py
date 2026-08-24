#!/usr/bin/env python3
"""Generate MinIO IAM policy JSON documents: one per (team, tier).

Design constraints (fixes applied):
  1. No shared/common "view" policy with an explicit Deny attached
     alongside dev/adm policies on the same principal. Each policy here is
     scoped to exactly one team+tier and is purely additive (Allow-only).
     Write/delete restriction for lower tiers comes from the *absence* of
     an Allow (implicit deny), never from an explicit Deny statement — so
     attaching a view-tier policy can never cancel a dev/adm-tier Allow
     attached to the same principal.
  2. admin:CreateServiceAccount is granted to the adm tier only, and never
     explicitly Denied to dev/view — MinIO already permits any
     authenticated identity to create a service account *for itself*
     without needing this permission; only creating one *for another user*
     needs it. Denying the action outright would break self-service-account
     creation for every tier, including the LDAP user's own login flow.
  3. Each (team, tier) pair maps to exactly one policy name; the caller is
     responsible for assigning at most one tier per person per team (see
     grants.yaml-style validation note below) — assigning the same person
     to both <team>-dev and <team>-adm would let dev's explicit... (n/a,
     see #1: there is no explicit Deny here to begin with, so this
     particular conflict class doesn't arise with this policy set. Kept as
     a validation reminder for whatever grants/team-assignment file drives
     LDAP group membership, since a future revision might reintroduce an
     explicit Deny somewhere.)

Edit TEAMS below to match real bucket names, then re-run.
"""
import json
import os

TEAMS = {
    "bi": ["team-bi"],
    "ml": ["team-ml"],
    "ops": ["team-ops"],
}

OUT_DIR = os.path.join(os.path.dirname(__file__), "policies")


def bucket_resources(buckets):
    res = []
    for b in buckets:
        res.append(f"arn:aws:s3:::{b}")
        res.append(f"arn:aws:s3:::{b}/*")
    return res


def object_resources(buckets):
    return [f"arn:aws:s3:::{b}/*" for b in buckets]


def bucket_only_resources(buckets):
    return [f"arn:aws:s3:::{b}" for b in buckets]


def policy_view(buckets):
    return {
        "Version": "2012-10-17",
        "Statement": [
            {
                "Effect": "Allow",
                "Action": ["s3:GetObject", "s3:GetObjectTagging"],
                "Resource": object_resources(buckets),
            },
            {
                "Effect": "Allow",
                "Action": ["s3:ListBucket", "s3:GetBucketLocation"],
                "Resource": bucket_only_resources(buckets),
            },
        ],
    }


def policy_dev(buckets):
    p = policy_view(buckets)
    p["Statement"].append(
        {
            "Effect": "Allow",
            "Action": [
                "s3:PutObject",
                "s3:PutObjectTagging",
                "s3:PutLifecycleConfiguration",
                "s3:GetLifecycleConfiguration",
            ],
            "Resource": object_resources(buckets) + bucket_only_resources(buckets),
        }
    )
    # No DeleteObject Allow anywhere in this policy: deletion is blocked by
    # implicit deny, not an explicit Deny statement (fix #1).
    #
    # NOTE (judgment call, flagged not auto-decided): PutLifecycleConfiguration
    # lets a dev set an ILM rule like "Expiration: Days 1", which achieves
    # deletion indirectly even though s3:DeleteObject itself is never
    # granted. If blocking deletion is the actual goal for this tier,
    # move PutLifecycleConfiguration to the adm tier instead — left in dev
    # here because that's what was specified, not because it's clearly
    # correct.
    return p


def policy_adm(buckets):
    p = policy_dev(buckets)
    p["Statement"].append(
        {
            "Effect": "Allow",
            "Action": ["s3:DeleteObject", "s3:PutBucketPolicy", "s3:GetBucketPolicy"],
            "Resource": object_resources(buckets) + bucket_only_resources(buckets),
        }
    )
    p["Statement"].append(
        {
            "Effect": "Allow",
            "Action": ["admin:CreateServiceAccount"],
            "Resource": ["arn:aws:s3:::*"],
        }
    )
    # NOTE (judgment call, flagged not auto-decided): s3:PutBucketPolicy
    # lets an adm attach an anonymous/public-read bucket policy, which is
    # an actual data-exposure risk if this bucket ever holds anything
    # sensitive. If that needs blocking company-wide, this action belongs
    # on a separate, more tightly-controlled policy rather than bundled
    # into every team's adm tier.
    return p


TIER_BUILDERS = {"view": policy_view, "dev": policy_dev, "adm": policy_adm}


def main():
    os.makedirs(OUT_DIR, exist_ok=True)
    names = []
    for team, buckets in TEAMS.items():
        for tier, build in TIER_BUILDERS.items():
            name = f"{team}-{tier}"
            doc = build(buckets)
            path = os.path.join(OUT_DIR, f"{name}.json")
            with open(path, "w") as f:
                json.dump(doc, f, indent=2)
            names.append(name)
    print(f"generated {len(names)} policies in {OUT_DIR}:")
    for n in names:
        print(" -", n)


if __name__ == "__main__":
    main()
