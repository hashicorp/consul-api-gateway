schema_version = 1

project {
  license        = "MPL-2.0"
  copyright_year = 2021

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
