schema_version = 1

project {
  # (OPTIONAL) A list of globs that should not have copyright or license headers .
  # Supports doublestar glob patterns for more flexibility in defining which
  # files or folders should be ignored
  # Default: []
  header_ignore = [
    "config/rbac/role.yaml",
    "config/crd/bases/*.yaml",
    "**/testdata/**.golden.yaml"
  ]
}
