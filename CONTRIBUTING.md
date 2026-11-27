# Submitting Issues

We use GitHub issues to track public bugs. If you are submitting a bug, please provide
as much info as possible to make it easier to reproduce the issue and update unit tests.

# Contributing Code

## Setting up

The module needs Go 1.26 or later. Build, test and check with

    make build
    make test
    make check

`make check` runs `go mod tidy -diff`, `go vet`, golangci-lint (if installed), the changelog check
and codespell. Pre-commit hooks run `go fmt`, golangci-lint, the unit tests, codespell and the
changelog check, and check the commit message. Install them once per clone with

    make pre-commit-install

which creates a Python virtual environment in `.venv` with `pre-commit` and installs both the
pre-commit and the commit-msg hook.

## Pull requests

This project uses [conventional commits](https://www.conventionalcommits.org/en/v1.0.0/#summary).

We follow the [GitHub Flow](https://guides.github.com/introduction/flow/index.html) so all contributions happen through pull requests. We actively welcome your pull requests:

1.  Fork the repo and create your branch from main.
2.  If you've added code that should be tested, add tests.
3.  If you've changed APIs, update the documentation.
4.  Ensure the test suite passes.
5.  Issue that pull request!

When submitting code changes your submissions are understood to be under the same MIT License that covers the project. Feel free to contact Eyevinn Technology if that's a concern.

## Changelog

For each change that users will notice, add a file to [changelog.d](changelog.d/) instead of editing
[CHANGELOG.md](CHANGELOG.md), as described in [changelog.d/README.md](changelog.d/README.md). Keep
each entry to one or two lines that say what changed and name the affected API or tool. Separate
files never conflict, so pull requests can be merged in any order. The entries move into
CHANGELOG.md when a version is released.

## Releases

Maintainers prepare a release in a pull request `chore: prepare vX.Y.Z release` from a branch
`release-vX.Y.Z`. It moves the changelog entries into CHANGELOG.md as described in
[changelog.d/README.md](changelog.d/README.md). Before 1.0, a new API or a breaking change gives a
minor version and fixes only a patch version.

Tag `vX.Y.Z` only on a commit where `scripts/check-changelog.sh vX.Y.Z` passes. Programs built
from the tag report its version (`ttx-gen -version`). A tag is permanent for Go modules, since the
module proxy caches it, so never move or reuse one.

# Code of Conduct

## Our Pledge

In the interest of fostering an open and welcoming environment, we as contributors and maintainers pledge to making participation in our project and our community a harassment-free experience for everyone, regardless of age, body size, disability, ethnicity, gender identity and expression, level of experience, nationality, personal appearance, race, religion, or sexual identity and orientation.

## Our Standards

Examples of behavior that contributes to creating a positive environment include:

- Using welcoming and inclusive language
- Being respectful of differing viewpoints and experiences
- Gracefully accepting constructive criticism
- Focusing on what is best for the community
- Showing empathy towards other community members

Examples of unacceptable behavior by participants include:

- The use of sexualized language or imagery and unwelcome sexual attention or advances
- Trolling, insulting/derogatory comments, and personal or political attacks
- Public or private harassment
- Publishing others' private information, such as a physical or electronic address, without explicit permission
- Other conduct which could reasonably be considered inappropriate in a professional setting

## Our Responsibilities

Project maintainers are responsible for clarifying the standards of acceptable behavior and are expected to take appropriate and fair corrective action in response to any instances of unacceptable behavior.

Project maintainers have the right and responsibility to remove, edit, or reject comments, commits, code, wiki edits, issues, and other contributions that are not aligned to this Code of Conduct, or to ban temporarily or permanently any contributor for other behaviors that they deem inappropriate, threatening, offensive, or harmful.

## Scope

This Code of Conduct applies both within project spaces and in public spaces when an individual is representing the project or its community. Examples of representing a project or community include using an official project e-mail address, posting via an official social media account, or acting as an appointed representative at an online or offline event. Representation of a project may be further defined and clarified by project maintainers.

## Enforcement

Instances of abusive, harassing, or otherwise unacceptable behavior may be reported by contacting the project team. All complaints will be reviewed and investigated and will result in a response that is deemed necessary and appropriate to the circumstances. The project team is obligated to maintain confidentiality with regard to the reporter of an incident. Further details of specific enforcement policies may be posted separately.

Project maintainers who do not follow or enforce the Code of Conduct in good faith may face temporary or permanent repercussions as determined by other members of the project's leadership.

## Attribution

This Code of Conduct is adapted from the Contributor Covenant, version 1.4, available at http://contributor-covenant.org/version/1/4
