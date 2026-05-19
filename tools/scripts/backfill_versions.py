#!/usr/bin/env python3
# Copyright Jamf Software LLC 2026
# SPDX-License-Identifier: MIT
"""Backfill older-version Pro endpoints into tools/generate/config.json.

Jamf publishes endpoints under /v1/, /v2/, /v3/ paths and keeps prior
versions in the spec until they are physically removed. The SDK's policy
(issue #19) is to retain every spec version side-by-side so downstream
consumers (notably terraform-provider-jamfplatform) get a real migration
window when Jamf marks an endpoint deprecated.

This script reconciles config.json with the Pro spec by inserting any
spec version of a multi-version base path that is missing from config.
For each missing version it:

  - Synthesizes an OperationDef from a sibling already in config.
  - Adjusts the operation name's trailing V<n> suffix to the new version.
  - Replicates resolver / resolvers / apply blocks with version-suffixed
    resourceType / typedReturn / op-name / updateType / membershipPreFetch
    fields so each version has its own Resolve<X>V<N>ByName and
    Apply<X>V<N> sugar.
  - For V1 typedReturn lookups, falls back to the unsuffixed schema name
    when present (Jamf's V1 schemas often lack a version suffix —
    `ComputerInventory` instead of `ComputerInventoryV1`).

ExpectedStatus is deliberately not carried over: it varies between
versions (e.g. PATCH detail returns 200 on V1 but 204 on V2/V3) and the
generator's detectResponse infers it from each spec operation directly.

Run from the repo root: `python3 tools/scripts/backfill_versions.py`,
then re-run `make generate` to materialise the new methods.
"""
import json, re, sys
from pathlib import Path
from collections import defaultdict

REPO = Path(__file__).resolve().parents[2]
CFG = REPO / 'tools' / 'generate' / 'config.json'
SPEC = REPO / 'testing' / 'openapi-jpapi.json'

with open(CFG) as f:
    cfg = json.load(f)
with open(SPEC) as f:
    spec = json.load(f)

schemas = spec.get('components', {}).get('schemas', {})
pro_spec = next(s for s in cfg['specs'] if s.get('package') == 'pro')


def parse_op(s):
    m = re.match(r'^(\w+)\s+(.+)$', s)
    return (m.group(1).upper(), m.group(2))


cfg_ops_by_key = {}
cfg_ops_by_name = {}
for op in pro_spec['operations']:
    k = parse_op(op['op'])
    cfg_ops_by_key[k] = op
    cfg_ops_by_name[op['name']] = op

ver_by_base = defaultdict(set)
for p in spec['paths']:
    m = re.match(r'^/v([0-9]+)(.*)$', p)
    if m:
        ver_by_base[m.group(2)].add(int(m.group(1)))

multi_bases = {b: sorted(vs) for b, vs in ver_by_base.items() if len(vs) > 1}


def adjust_type_name(name: str, target_v: int) -> str:
    if not name:
        return name
    m = re.search(r'V(\d+)$', name)
    if m:
        base = name[:-len(m.group(0))]
        if target_v == 1:
            if base in schemas:
                return base
            return f'{base}V{target_v}'
        return f'{base}V{target_v}'
    if target_v == 1:
        return name
    return f'{name}V{target_v}'


def adjust_op_name(name: str, target_v: int) -> str:
    m = re.search(r'V(\d+)$', name)
    if not m:
        return f'{name}V{target_v}'
    return name[:-len(m.group(0))] + f'V{target_v}'


def adjust_resolver(r: dict, target_v: int) -> dict:
    out = {}
    for k, v in r.items():
        if k == 'resourceType':
            mm = re.search(r'V(\d+)$', v)
            out[k] = (v[:-len(mm.group(0))] if mm else v) + f'V{target_v}'
        elif k == 'typedReturn':
            out[k] = adjust_type_name(v, target_v)
        elif k == 'apply':
            ap = {}
            for ak, av in v.items():
                if ak in ('createOp', 'updateOp', 'deleteOp', 'getOp', 'tokenUploadCreateOp', 'tokenReplaceOp'):
                    ap[ak] = adjust_op_name(av, target_v)
                elif ak == 'updateType':
                    ap[ak] = adjust_type_name(av, target_v)
                elif ak == 'membershipPreFetch':
                    mpf = dict(av)
                    if 'fetchOp' in mpf:
                        mpf['fetchOp'] = adjust_op_name(mpf['fetchOp'], target_v)
                    if 'assignmentType' in mpf:
                        mpf['assignmentType'] = adjust_type_name(mpf['assignmentType'], target_v)
                    ap[ak] = mpf
                else:
                    ap[ak] = av
            out[k] = ap
        else:
            out[k] = v
    return out


def spec_query_param_names(target_path: str, method: str) -> set:
    """Return the set of query parameter names declared on the spec op
    at target_path / method. Empty set if op or params missing."""
    op = spec['paths'].get(target_path, {}).get(method.lower(), {})
    return {p.get('name') for p in op.get('parameters', []) if p.get('in') == 'query' and p.get('name')}


def filter_params_for_version(sibling_params: list, target_path: str, method: str) -> list:
    """Drop entries from the sibling's params list whose query-name is
    not present on the target version's spec op. Param string format is
    "name", "name:type", or "name:type:goName" — first segment is the
    spec name."""
    if not sibling_params:
        return sibling_params
    allowed = spec_query_param_names(target_path, method)
    kept = []
    for p in sibling_params:
        name = p.split(':', 1)[0]
        if name in allowed:
            kept.append(p)
    return kept


def synthesize_op(sibling: dict, target_v: int, target_path: str) -> dict:
    new = {}
    method, _ = parse_op(sibling['op'])
    new['op'] = f'{method} {target_path}'
    new['name'] = adjust_op_name(sibling['name'], target_v)
    for k in ('pagination', 'pageSizeParam', 'contentType', 'params', 'unwrapResults', 'requestType', 'responseType', 'pathNames'):
        if k not in sibling:
            continue
        if k == 'params':
            new[k] = filter_params_for_version(sibling[k], target_path, method)
            if not new[k]:
                del new[k]
        else:
            new[k] = sibling[k]
    # Disable pagination when the target op's response is a raw array
    # rather than a paginated envelope. The generator's paginators expect
    # a totalCount/hasNext envelope; passing one to a raw-array op makes
    # the server reject the synthetic page-size query.
    if 'pagination' in new and is_raw_array_response(target_path, method):
        del new['pagination']
        new.pop('pageSizeParam', None)
    if 'resolver' in sibling:
        new['resolver'] = adjust_resolver(sibling['resolver'], target_v)
    if 'resolvers' in sibling:
        new['resolvers'] = [adjust_resolver(r, target_v) for r in sibling['resolvers']]
    return new


def is_raw_array_response(target_path: str, method: str) -> bool:
    """True when the spec's 200/2xx response body for target_path/method
    is a top-level JSON array rather than an object envelope."""
    op = spec['paths'].get(target_path, {}).get(method.lower(), {})
    for status, resp in op.get('responses', {}).items():
        if not status.startswith('2'):
            continue
        for ct in ('application/json', '*/*'):
            sch = resp.get('content', {}).get(ct, {}).get('schema')
            if sch and sch.get('type') == 'array':
                return True
    return False


ops_list = pro_spec['operations']
inserted = 0
for base in sorted(multi_bases.keys()):
    vers = multi_bases[base]
    for v in sorted(vers):
        full = f'/v{v}{base}'
        path_item = spec['paths'].get(full, {})
        for httpm, op in path_item.items():
            if httpm not in ('get', 'post', 'put', 'patch', 'delete'):
                continue
            key = (httpm.upper(), full)
            if key in cfg_ops_by_key:
                continue
            sibling = None
            for v2 in sorted(vers, reverse=True):
                if v2 == v:
                    continue
                k2 = (httpm.upper(), f'/v{v2}{base}')
                if k2 in cfg_ops_by_key:
                    candidate = cfg_ops_by_key[k2]
                    if candidate not in ops_list:
                        continue
                    sibling = candidate
                    break
            if not sibling:
                print(f'WARN: no sibling found for {httpm.upper()} {full}', file=sys.stderr)
                continue
            new_op = synthesize_op(sibling, v, full)
            if new_op['name'] in cfg_ops_by_name:
                print(f'SKIP: name collision {new_op["name"]}', file=sys.stderr)
                continue
            idx = ops_list.index(sibling)
            ops_list.insert(idx, new_op)
            cfg_ops_by_name[new_op['name']] = new_op
            cfg_ops_by_key[parse_op(new_op['op'])] = new_op
            inserted += 1

print(f'inserted {inserted} ops')

with open(CFG, 'w') as f:
    json.dump(cfg, f, indent=2, ensure_ascii=False)
    f.write('\n')
