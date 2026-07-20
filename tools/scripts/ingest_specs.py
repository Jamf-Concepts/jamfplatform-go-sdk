#!/usr/bin/env python3
# Copyright Jamf Software LLC 2026
# SPDX-License-Identifier: MIT
"""Ingest SDK source specs from a Jamf Platform APIs GitOps archive (zip).

The generator's private source specs (the gitignored `testing/` directory)
were historically sourced by hand, one file per API family. This script
replaces that manual step: point it at a single GitOps archive
(`jamf-platform-apis-gitops-vNNN-*.zip`, produced from jamf/public-apis-oas)
and it drops the correct spec for each family into `testing/` under the exact
filenames `tools/generate/config.json` already expects. No config change is
needed — after ingest, run `make generate` as usual.

    python3 tools/scripts/ingest_specs.py --zip path/to/archive.zip
    make generate

Which archive member feeds which family
---------------------------------------
The archive ships two environments (`external/` = production gateway,
`internal/{dev,stage}/`) and, per family, a production dir plus one or more
`-beta` dirs. Two facts drive the mapping:

  * Only the *beta* variants carry the path shape the SDK config whitelists
    (`/v1/tenant/{tenantId}/...`). The production variants have the tenant
    segment stripped, so every operation lookup would miss and the generator
    would emit nothing.
  * Only the *beta* variants carry the `x-required-privileges` extension that
    becomes the SDK's `Required privileges:` godoc and per-package `Privileges`
    registries. The production variants have zero.

So each family resolves to its privilege-bearing beta variant. The selection
is automatic: for family F we try `F-beta-beta`, then `F-beta`, then `F`, and
pick the first that exists AND carries privileges. That rule handles the one
naming quirk in the archive — compliance-benchmarks, whose privilege-bearing
spec is `compliance-benchmarks-beta-beta` (plain `compliance-benchmarks-beta`
has none) — without special-casing, and stays correct if a future build
renames the variants. A privilege-bearing family that resolves to zero
privileges is treated as a hard error (wrong archive / changed structure).

Exceptions (not sourced from the archive)
------------------------------------------
  * App Installer Titles / Deployments / Global Settings — these three `pro`
    specs are NOT present in the archive (the app-installers paths are absent
    from jpapi too). They remain manually sourced in `testing/`. The generator
    falls back to their committed `api/*.json` when the `testing/` file is
    absent (see resolveSpecPath in tools/generate/emit.go), so they keep
    working regardless.
  * jpapi version — the archive publishes whatever Jamf Pro API version the
    upstream jamf/public-apis-oas repo has released, which historically lags
    the shipping Jamf Pro version by a minor. Adopting an archive therefore
    pins jpapi to that build's version. Check `JamfProAPIVersion` after
    `make generate` and confirm the version is acceptable before committing.

Why CI does not need this script
--------------------------------
CI regenerates Go from the committed `api/*.json` published specs via the
generator's fallback path, never from `testing/`. This script only refreshes
the private `testing/` sources a maintainer uses to regenerate the public
`api/*` surface when it changes.
"""

import argparse
import json
import os
import sys
import zipfile

try:
    import yaml
except ImportError:
    sys.exit(
        "PyYAML is required (archive specs are YAML). Install with: pip install pyyaml"
    )


# dest filename in testing/  ->  archive family directory stem.
# Candidate variants (family-beta-beta, family-beta, family) are derived from
# the stem; the privilege-bearing one is selected automatically.
FAMILY_MAP = {
    "openapi-jpapi.json": "jpapi",
    "Classic-openapi.json": "capi",
    "blueprints-api.json": "blueprints",
    "device-groups-api.json": "device-groups",
    "device-inventory-api.json": "devices",
    "device-management-actions-api.json": "device-management-action",
    "Declaration-reporting-openapi.json": "declaration-reporting",
    "jamf-compliance-benchmark-engine-api.yaml": "compliance-benchmarks",
}

# Sourced manually, not from the archive. Listed only so the summary is honest
# about the full picture; these files are never touched.
MANUAL_EXCEPTIONS = {
    "AppInstallerTitles.yaml": "app-installers paths are not in the archive",
    "AppInstallerDeployments.yaml": "app-installers paths are not in the archive",
    "AppInstallerGlobalSettings.yaml": "app-installers paths are not in the archive",
}

PRIV_KEY = "x-required-privileges"


def find_member(names, env, variant):
    """Return the archive member for env/variant/openapi.yaml, tolerating an
    optional top-level wrapper directory. None if absent."""
    needle = f"{env}/{variant}/openapi.yaml"
    for n in names:
        if n == needle or n.endswith("/" + needle):
            return n
    return None


def path_count(doc):
    return len(doc.get("paths") or {})


def priv_count(raw_text):
    # Count occurrences the same way `grep -c` does on the raw spec — robust to
    # exactly where the extension is nested.
    return raw_text.count(PRIV_KEY)


def resolve(zf, names, env, family):
    """Pick the best variant for a family. Returns (variant, member, doc,
    raw_bytes, npaths, nprivs) or raises."""
    candidates = [f"{family}-beta-beta", f"{family}-beta", family]
    seen = []
    fallback = None  # first existing candidate, used only if none carry privs
    for variant in candidates:
        member = find_member(names, env, variant)
        if member is None:
            continue
        raw = zf.read(member)
        text = raw.decode("utf-8", "replace")
        doc = yaml.safe_load(text)
        npaths, nprivs = path_count(doc), priv_count(text)
        seen.append(f"{variant}(paths={npaths}, privs={nprivs})")
        if fallback is None:
            fallback = (variant, member, doc, raw, npaths, nprivs)
        if nprivs > 0:
            return (variant, member, doc, raw, npaths, nprivs)
    if fallback is None:
        raise SystemExit(
            f"family {family!r}: no variant found under {env}/ "
            f"(tried {', '.join(candidates)})"
        )
    # Existed but zero privileges across every candidate — wrong archive/env or
    # the structure changed. Fail loudly rather than emit privilege-less code.
    raise SystemExit(
        f"family {family!r}: found variants but none carry {PRIV_KEY} "
        f"[{', '.join(seen)}]. Wrong environment ({env})? Production variants "
        f"have no privileges — use --env external and beta variants."
    )


def main():
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--zip", required=True, help="path to the GitOps archive (.zip)")
    ap.add_argument(
        "--env",
        default="external",
        choices=["external", "internal/dev", "internal/stage"],
        help="environment tree within the archive (default: external = production gateway)",
    )
    ap.add_argument(
        "--dest",
        default="testing",
        help="destination spec directory (default: testing)",
    )
    ap.add_argument(
        "--dry-run",
        action="store_true",
        help="resolve and report but write nothing",
    )
    args = ap.parse_args()

    if not os.path.isfile(args.zip):
        sys.exit(f"archive not found: {args.zip}")

    rows = []
    with zipfile.ZipFile(args.zip) as zf:
        names = zf.namelist()
        for dest, family in FAMILY_MAP.items():
            variant, member, doc, raw, npaths, nprivs = resolve(
                zf, names, args.env, family
            )
            out_path = os.path.join(args.dest, dest)
            if not args.dry_run:
                os.makedirs(args.dest, exist_ok=True)
                if dest.endswith(".json"):
                    with open(out_path, "w", encoding="utf-8") as f:
                        json.dump(doc, f, ensure_ascii=False, indent=2)
                        f.write("\n")
                else:  # .yaml — write the archive bytes verbatim
                    with open(out_path, "wb") as f:
                        f.write(raw)
            rows.append((dest, variant, npaths, nprivs))

    verb = "Would ingest" if args.dry_run else "Ingested"
    print(f"{verb} from {args.zip} (env: {args.env})\n")
    w = max(len(r[0]) for r in rows)
    sources = [f"{variant}/openapi.yaml" for _, variant, _, _ in rows]
    sw = max(len("source variant"), max(len(s) for s in sources))
    print(f"  {'dest':<{w}}  {'source variant':<{sw}}  paths  privs")
    print(f"  {'-'*w}  {'-'*sw}  -----  -----")
    for (dest, _variant, npaths, nprivs), src in zip(rows, sources):
        print(f"  {dest:<{w}}  {src:<{sw}}  {npaths:>5}  {nprivs:>5}")
    print("\n  manual (not in archive, untouched):")
    for f, why in MANUAL_EXCEPTIONS.items():
        print(f"    {f:<{w}}  — {why}")

    if not args.dry_run:
        print(
            "\nNext: run `make generate`, then confirm JamfProAPIVersion in "
            "jamfplatform/version.go is the version you expect before committing."
        )


if __name__ == "__main__":
    main()
