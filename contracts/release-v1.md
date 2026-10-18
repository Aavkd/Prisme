# Release manifest v1

A successful, explicitly configured GitHub Actions workflow publishes an artifact
named `prisme-release-${GITHUB_RUN_ATTEMPT}`. Its ZIP contains exactly one root file,
`prisme-release.json`. The file is bounded to 64 KiB; the archive to 1 MiB.

```json
{
  "version": 1,
  "repository": "YOUR_OWNER/sounding-lines",
  "commit": "0123456789012345678901234567890123456789",
  "run_id": "123456",
  "run_attempt": 1,
  "environment": "production",
  "images": [
    {
      "service": "backend",
      "reference": "ghcr.io/YOUR_LOWERCASE_OWNER/backend@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
      "tag": "v1.2.3",
      "provenance": ""
    }
  ]
}
```

The image above is illustrative. Use the actual lowercase registry/repository and
the digest returned by the successful image push, including a manifest-list digest
for a multi-platform image. Never substitute a tag, a commit SHA, an artifact ZIP
digest, or an invented digest. `tag` and `provenance` may be empty strings. Include
all service images produced by this release; service names must be unique.

The importer verifies the configured repository ID, repository name, publishing
workflow ID, successful completion, same-repository head, eligible trigger (`push`,
`workflow_dispatch`, `release`), commit, run ID, attempt, target environment and
immutable image-reference syntax. Pull-request/fork builds remain visible but
cannot create production release records. When GitHub supplies an artifact digest,
the archive is checked against it. The manifest is evidence from the configured
workflow; it is **not** registry availability verification or a verified signature
attestation. An optional provenance string is presented as supplied evidence only.

Release identity is stable per repository/run/attempt. Existing release records
cannot be overwritten, and a rerun does not inherit earlier provenance. A missing,
expired, oversized, invalid or mismatching artifact leaves `release_state` as
`unavailable`. Other runs continue to synchronize. A later poll retries the import.

Workflow integration after an existing successful push (adapt variable names):

```yaml
- name: Write Prisme release metadata
  env:
    IMAGE: ghcr.io/YOUR_LOWERCASE_OWNER/backend
    DIGEST: ${{ steps.build.outputs.digest }}
  run: |
    python3 - <<'PY'
    import json, os, re
    digest = os.environ['DIGEST']
    assert re.fullmatch(r'sha256:[a-f0-9]{64}', digest)
    manifest = {
      'version': 1, 'repository': os.environ['GITHUB_REPOSITORY'],
      'commit': os.environ['GITHUB_SHA'], 'run_id': os.environ['GITHUB_RUN_ID'],
      'run_attempt': int(os.environ['GITHUB_RUN_ATTEMPT']),
      'environment': 'production',
      'images': [{'service': 'backend', 'reference': os.environ['IMAGE']+'@'+digest,
                  'tag': '', 'provenance': ''}],
    }
    with open('prisme-release.json', 'w') as f:
      json.dump(manifest, f)
    PY
- uses: actions/upload-artifact@v4
  with:
    name: prisme-release-${{ github.run_attempt }}
    path: prisme-release.json
    if-no-files-found: error
```

This fragment is a template, not an installed or executed pilot workflow. It must
follow a push-enabled image build and must not be appended to a PR-only build.
Workflow publication permissions belong to that repository's workflow and are
separate from Prisme's read-only GitHub App permissions.
