#!/usr/bin/env python3
"""Sanity-check api/openapi.yaml without any OpenAPI library.

Checks:
  - the file parses and declares OpenAPI 3.x
  - every local $ref resolves
  - operationIds are unique and every operation has a response
  - every {path} parameter is declared
  - Site type and state enums match the CHECK constraints in migrations/0001_core.sql,
    so the API and the database cannot drift apart silently
Needs PyYAML (pip install pyyaml).
"""
import pathlib
import re
import sys

import yaml

ROOT = pathlib.Path(__file__).resolve().parent.parent
SPEC = ROOT / "api" / "openapi.yaml"
SCHEMA = ROOT / "migrations" / "0001_core.sql"
METHODS = {"get", "put", "post", "delete", "patch", "options", "head"}


def walk(node):
    if isinstance(node, dict):
        yield node
        for v in node.values():
            yield from walk(v)
    elif isinstance(node, list):
        for v in node:
            yield from walk(v)


def resolve(doc, ref):
    if not ref.startswith("#/"):
        return None
    cur = doc
    for part in ref[2:].split("/"):
        if not isinstance(cur, dict) or part not in cur:
            return None
        cur = cur[part]
    return cur


def sql_enum(sql, column_pattern):
    """Values from `<column> TEXT NOT NULL CHECK (<column> IN (...))` inside CREATE TABLE sites."""
    table = re.search(r"CREATE TABLE sites \((.*?)\n\);", sql, re.S)
    if not table:
        return None
    m = re.search(column_pattern + r"\s+TEXT NOT NULL CHECK \(\w+ IN\s*\((.*?)\)\)", table.group(1), re.S)
    if not m:
        return None
    return re.findall(r"'([^']+)'", m.group(1))


def main() -> int:
    errors = []
    doc = yaml.safe_load(SPEC.read_text())

    if not str(doc.get("openapi", "")).startswith("3."):
        errors.append("openapi version must be 3.x")

    for node in walk(doc):
        ref = node.get("$ref") if isinstance(node, dict) else None
        if ref and resolve(doc, ref) is None:
            errors.append(f"unresolved $ref {ref}")

    seen = {}
    for path, item in doc.get("paths", {}).items():
        shared = item.get("parameters", [])
        for method, op in item.items():
            if method not in METHODS:
                continue
            where = f"{method.upper()} {path}"
            op_id = op.get("operationId")
            if not op_id:
                errors.append(f"{where}: missing operationId")
            elif op_id in seen:
                errors.append(f"{where}: operationId {op_id} also used by {seen[op_id]}")
            else:
                seen[op_id] = where
            if not op.get("responses"):
                errors.append(f"{where}: no responses")
            declared = set()
            for p in shared + op.get("parameters", []):
                p = resolve(doc, p["$ref"]) if "$ref" in p else p
                if p and p.get("in") == "path":
                    declared.add(p["name"])
            for name in re.findall(r"{(\w+)}", path):
                if name not in declared:
                    errors.append(f"{where}: path parameter {name} is not declared")

    sql = SCHEMA.read_text()
    schemas = doc["components"]["schemas"]
    for schema_name, column in (("SiteType", r"\btype"), ("SiteState", r"\bstate")):
        from_sql = sql_enum(sql, column)
        from_api = schemas[schema_name].get("enum")
        if from_sql is None:
            errors.append(f"could not read {schema_name} values from {SCHEMA.name}")
        elif sorted(from_sql) != sorted(from_api or []):
            errors.append(f"{schema_name} differs: api={sorted(from_api or [])} sql={sorted(from_sql)}")

    print(f"{len(seen)} operations, {len(schemas)} schemas")
    for e in errors:
        print("FAIL", e, file=sys.stderr)
    return 1 if errors else 0


if __name__ == "__main__":
    sys.exit(main())
