#!/usr/bin/env bash

set -e

changelog_file_path=$1

if [ -n "$2" ]; then
  enforce_matching_pull_request_number="matching this PR number "
fi

# Check if there is a diff matching the expected changelog file path
changelog_files=$(git --no-pager diff --name-only HEAD "$(git merge-base HEAD "origin/main")" -- ${changelog_file_path})

# Exit with error if no changelog entry is found
if [ -z "$changelog_files" ]; then
  echo "Did not find a changelog entry ${enforce_matching_pull_request_number}and the 'pr/no-changelog' label was not applied. Reference - https://github.com/hashicorp/consul/pull/8387"
  exit 1
fi

# Validate format with make changelog-check, exit with error if any note has an
# invalid format
for file in $changelog_files; do
  if ! cat $file | make changelog-check; then
    echo "Found a changelog entry ${enforce_matching_pull_request_number}but the note format in ${file} was invalid."
    exit 1
  fi
done

echo "Found valid changelog entry!"
