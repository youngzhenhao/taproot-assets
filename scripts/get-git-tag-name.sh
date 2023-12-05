#!/bin/bash

# This script derives a git tag name from the version fields found in a given Go
# file. It also checks if the derived git tag name is a valid SemVer compliant
# version string.

# get_git_tag_name reads the version fields from the given file and then
# constructs and returns a git tag name.
get_git_tag_name() {
  local file_path="$1"

  # Check if the file exists
  if [ ! -f "$file_path" ]; then
      echo "Error: File not found at $file_path" >&2
      exit 1
  fi

  # Read and parse the version fields.
  local app_major
  app_major=$(grep -oP 'AppMajor\s*uint\s*=\s*\K\d+' "$file_path")

  local app_minor
  app_minor=$(grep -oP 'AppMinor\s*uint\s*=\s*\K\d+' "$file_path")

  local app_patch
  app_patch=$(grep -oP 'AppPatch\s*uint\s*=\s*\K\d+' "$file_path")

  local app_status
  app_status=$(grep -oP 'AppStatus\s*=\s*"([a-z]*)"' "$file_path" | cut -d '"' -f 2)

  local app_pre_release
  app_pre_release=$(grep -oP 'AppPreRelease\s*=\s*"([a-z0-9]*)"' "$file_path" | cut -d '"' -f 2)

  # Parse the GitTagIncludeStatus field.
  gitTagIncludeStatus=false

  if grep -q 'GitTagIncludeStatus = true' "$file_path"; then
      gitTagIncludeStatus=true
  elif grep -q 'GitTagIncludeStatus = false' "$file_path"; then
      gitTagIncludeStatus=false
  else
      echo "Error: GitTagIncludeStatus is not present in the file."
      exit 1
  fi

  # Construct the git tag name with conditional inclusion of app_status and
  # app_pre_release
  tag_name="v${app_major}.${app_minor}.${app_patch}"

  # Append app_status if gitTagIncludeStatus is true and app_status is not empty
  if [ "$gitTagIncludeStatus" = true ] && [ -n "$app_status" ]; then
      tag_name+="-${app_status}"
  fi

  # Append app_pre_release if it is not empty
  if [ -n "$app_pre_release" ]; then
      tag_name+="-${app_pre_release}"
  fi

  echo "$tag_name"
}

# check_git_tag_name checks if the given git tag name is a valid SemVer
# compliant version string.
check_git_tag_name() {
  local tag_name="$1"

  # This regex pattern ensures that a version string strictly adheres to the
  # SemVer 2.0.0 specification.
  #
  # Regex pattern explanation:
  # 1. ^(0|[1-9]\d*) - Major version: non-negative integer, no leading zero
  #    unless 0.
  # 2. \.(0|[1-9]\d*) - Minor version: same rules as major version.
  # 3. \.(0|[1-9]\d*) - Patch version: same rules as major version.
  # 4. Pre-release version matching:
  # 4.0. (- - This introduces the optional pre-release section, starting with
  #      a hyphen.
  # 4.1. (0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*) - Matches a pre-release
  #      identifier which can be:
  #      4.1.0. 0 or any number without leading zeros (e.g., 1, 123).
  #      4.1.1. An alphanumeric identifier (can include hyphens), not starting
  #             with a digit (e.g., alpha, beta-1).
  # 4.2. (\.(0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))* - This part allows
  #      multiple pre-release identifiers, separated by dots. It follows the
  #      same pattern as 4.1, allowing a series like 'alpha.beta.1'.
  # 4.3. )? - Closes the optional pre-release section, making this entire
  #      segment optional in the version string.
  # 5. Build metadata matching:
  # 5.0. (\+ - This introduces the optional build metadata section, starting
  #      with a plus sign.
  # 5.1. [0-9a-zA-Z-]+ - Matches a build metadata identifier. This can include:
  #      5.1.0. Numeric characters (0-9).
  #      5.1.1. Alphabetic characters, both uppercase (A-Z) and lowercase (a-z).
  #      5.1.2. Hyphens (-) within the identifier.
  # 5.2. (\.[0-9a-zA-Z-]+)* - Allows multiple build metadata identifiers,
  #      separated by dots. Each identifier follows the pattern in 5.1, enabling
  #      a series like '001.alpha.123'.
  # 5.3. )? - Closes the optional build metadata section, making this entire
  #      segment optional in the version string.
  # 6. $ - Ensures the entire string matches the pattern.
  semver_regex="^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(-((0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(\.(0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(\+([0-9a-zA-Z-]+(\.[0-9a-zA-Z-]+)*))?$"

  # Remove any leading 'v' from the tag name. This is a no-op if a 'v' prefix is
  # not present.
  tag_name="${tag_name#v}"

  if [[ $tag_name =~ $semver_regex ]]; then
    echo "Git tag name is a SemVer compliant version string" >&2
  else
    echo "Git tag name is not a SemVer compliant version string: $tag_name" >&2
    exit 1
  fi
}

file_path="$1"
echo "Reading version fields from $file_path" >&2
tag_name=$(get_git_tag_name "$file_path")
echo "Derived git tag name: $tag_name" >&2

check_git_tag_name "$tag_name"
echo "$tag_name"
