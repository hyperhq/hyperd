#!/bin/bash

# -----------------------------------------------------------------------------
# Version management helpers.  These functions help to set, save and load the
# following variables:
#
#    HYPER_GIT_COMMIT - The git commit id corresponding to this
#          source code.
#    HYPER_GIT_TREE_STATE - "clean" indicates no changes since the git commit id
#        "dirty" indicates source code changes after the git commit id
#    HYPER_GIT_VERSION - "vX.Y" used to indicate the last release version.
#    HYPER_GIT_MAJOR - The major part of the version
#    HYPER_GIT_MINOR - The minor component of the version

# Grovels through git to set a set of env variables.
#
# If HYPER_GIT_VERSION_FILE, this function will load from that file instead of
# querying git.
hyper::version::get_version_vars() {
  if [[ -n ${HYPER_GIT_VERSION_FILE-} ]]; then
    hyper::version::load_version_vars "${HYPER_GIT_VERSION_FILE}"
    return
  fi

  local git=(git --work-tree "${HYPER_ROOT}")

  if [[ -n ${HYPER_GIT_COMMIT-} ]] || HYPER_GIT_COMMIT=$("${git[@]}" rev-parse "HEAD^{commit}" 2>/dev/null); then
    if [[ -z ${HYPER_GIT_TREE_STATE-} ]]; then
      # Check if the tree is dirty.  default to dirty
      if git_status=$("${git[@]}" status --porcelain 2>/dev/null) && [[ -z ${git_status} ]]; then
        HYPER_GIT_TREE_STATE="clean"
      else
        HYPER_GIT_TREE_STATE="dirty"
      fi
    fi

    # Use git describe to find the version based on annotated tags.
    if [[ -n ${HYPER_GIT_VERSION-} ]] || HYPER_GIT_VERSION=$("${git[@]}" describe --tags --abbrev=14 "${HYPER_GIT_COMMIT}^{commit}" 2>/dev/null); then
      # This translates the "git describe" to an actual semver.org
      # compatible semantic version that looks something like this:
      #   v1.1.0-alpha.0.6+84c76d1142ea4d
      #
      # TODO: We continue calling this "git version" because so many
      # downstream consumers are expecting it there.
      HYPER_GIT_VERSION=$(echo "${HYPER_GIT_VERSION}" | sed "s/-\([0-9]\{1,\}\)-g\([0-9a-f]\{14\}\)$/.\1\+\2/")
      if [[ "${HYPER_GIT_TREE_STATE}" == "dirty" ]]; then
        # git describe --dirty only considers changes to existing files, but
        # that is problematic since new untracked .go files affect the build,
        # so use our idea of "dirty" from git status instead.
        HYPER_GIT_VERSION+="-dirty"
      fi


      # Try to match the "git describe" output to a regex to try to extract
      # the "major" and "minor" versions and whether this is the exact tagged
      # version or whether the tree is between two tagged versions.
      if [[ "${HYPER_GIT_VERSION}" =~ ^v([0-9]+)\.([0-9]+)(\.[0-9]+)?([-].*)?$ ]]; then
        HYPER_GIT_MAJOR=${BASH_REMATCH[1]}
        HYPER_GIT_MINOR=${BASH_REMATCH[2]}
        if [[ -n "${BASH_REMATCH[4]}" ]]; then
          HYPER_GIT_MINOR+="+"
        fi
      fi
    fi
  fi
}

# Saves the environment flags to $1
hyper::version::save_version_vars() {
  local version_file=${1-}
  [[ -n ${version_file} ]] || {
    echo "!!! Internal error.  No file specified in hyper::version::save_version_vars"
    return 1
  }

  cat <<EOF >"${version_file}"
HYPER_GIT_COMMIT='${HYPER_GIT_COMMIT-}'
HYPER_GIT_TREE_STATE='${HYPER_GIT_TREE_STATE-}'
HYPER_GIT_VERSION='${HYPER_GIT_VERSION-}'
HYPER_GIT_MAJOR='${HYPER_GIT_MAJOR-}'
HYPER_GIT_MINOR='${HYPER_GIT_MINOR-}'
EOF
}

# Loads up the version variables from file $1
hyper::version::load_version_vars() {
  local version_file=${1-}
  [[ -n ${version_file} ]] || {
    echo "!!! Internal error.  No file specified in hyper::version::load_version_vars"
    return 1
  }

  source "${version_file}"
}

# golang 1.5 wants `-X key=val`, but golang 1.4- REQUIRES `-X key val`
hyper::version::ldflag() {
  local key=${1}
  local val=${2}

  GO_VERSION=($(go version))

  if [[ -z $(echo "${GO_VERSION[2]}" | grep -E 'go1.5') ]]; then
    echo "-X ${HYPER_GO_PACKAGE}/pkg/version.${key} ${val}"
  else
    echo "-X ${HYPER_GO_PACKAGE}/pkg/version.${key}=${val}"
  fi
}

# Prints the value that needs to be passed to the -ldflags parameter of go build
# in order to set the hyperrnetes based on the git tree status.
hyper::version::ldflags() {
  hyper::version::get_version_vars

  local -a ldflags=()
  if [[ -n ${HYPER_GIT_COMMIT-} ]]; then
    ldflags+=($(hyper::version::ldflag "gitCommit" "${HYPER_GIT_COMMIT}"))
    ldflags+=($(hyper::version::ldflag "gitTreeState" "${HYPER_GIT_TREE_STATE}"))
  fi

  if [[ -n ${HYPER_GIT_VERSION-} ]]; then
    ldflags+=($(hyper::version::ldflag "gitVersion" "${HYPER_GIT_VERSION}"))
  fi

  if [[ -n ${HYPER_GIT_MAJOR-} && -n ${HYPER_GIT_MINOR-} ]]; then
    ldflags+=(
      $(hyper::version::ldflag "gitMajor" "${HYPER_GIT_MAJOR}")
      $(hyper::version::ldflag "gitMinor" "${HYPER_GIT_MINOR}")
    )
  fi

  # The -ldflags parameter takes a single string, so join the output.
  echo "${ldflags[*]-}"
}
