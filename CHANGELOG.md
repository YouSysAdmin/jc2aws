# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

## [4.3.0] 2026-08-31

#### Features:
- Migrate to Go 1.27.0

#### Fixed:
- aws: nil-pointer panics on empty profile name and nil Expiration,
  expiration now written as RFC3339 (botocore-compatible).

- cli: env vars no longer override explicit flags (J2A_SHELL*),
  SilenceUsage stops help dumps into eval'd stdout, cli-stdout no
  longer swallows errors, duration validated (900-43200) and output
  format/region validated before authentication.

- tui: unknown --account/--role-name now shows a warning instead of
  silently falling through; layout math guarded with min/max.

- jumpcloud: HTTP status checked on every step (xsrf/auth/IDP),
  MFA-required responses detected (ErrMFARequired), auth cookies
  propagated to the IDP request, step context in wrapped errors.

- totp: lenient secret parsing (padding/whitespace/dashes), empty
  secret is an error instead of a bogus code.

- mfa heuristic: exactly-6-digits = OTP code, everything else is a
  secret, inputs trimmed.

- update: zip directory entry can no longer produce a 0-byte binary,
  version-tag parse errors surfaced, checksum parser accepts */path
  formats, User-Agent on GitHub API, separate API/download timeouts,
  contexts on all requests, decompression-bomb caps, chmod after
  umask-affected writes.

- credentials/config files written atomically (temp+rename, 0600).

- config: ~ expansion, permissive-mode warning, sentinel errors
  (ErrAccountNotFound/ErrRoleNotFound), FindAccountByName no longer
  indexes a different slice than it searched.

- validators: https required for IdP URL, Get returns a safe no-op
  for unknown keys, underlying parse errors preserved.

#### Chore:
- Unused promptui/urfave deps
- Dead getters
- cyanStyle, isMasked
- unreachable code

## [4.2.0] 2026-05-26

#### Added
- Notification about successful login after exiting the TUI

## [4.1.0] 2026-04-09

#### Added
- Brew pkg release
  ```
  brew install yousysadmin/apps/jc2aws
  ```

## [4.0.1] 2026-04-03

#### Fixed
- Sidebar optimization: steps that will be auto-skipped (via config/flags/env) are hidden
  from the sidebar immediately after account selection, instead of showing as pending dots until the
  wizard sequentially reaches each one. (#19)[https://github.com/YouSysAdmin/jc2aws/issues/19]
- If retrieving credentials fails, user receives a "black screen" instead of seeing an error in the TUI.

#### Changed
- Update CHANGELOG
- Simplify CHANGELOG for the v4.0.0 - cosmetic changes have been removed.

## [4.0.0] 2026-04-02

#### Changed
- Migrate to the new TUI wizard using bubbletea, bubbles and lipgloss.

- Cobra and Viper are used to implement the CLI interface and configure.
  - Parameter precedence: CLI flag > environment variable (`J2A_*`) > config file defaults > account defaults > CLI defaults
  - Headless mode (default): all values from flags/env/config, no TUI launched
    - `-i` / `--interactive` flag (and `J2A_INTERACTIVE` env var) to launch the TUI wizard
    - `-s` / `--shell` flag as alias for `--output-format=shell`
    - `--shell-script` flag (and `J2A_SHELL_SCRIPT` env var) to run a script with AWS credential env vars

- Added new dependencies: `bubbletea`, `bubbles`, `lipgloss`, `cobra`, `viper`

- Updated `configs/jc2aws.yaml` example - added `aws_cli_profile`, `no_update_check`, `default_format`, `tui_done_action` fields.

- Updated `README.md` - added environment variables table, self-update section, and updated config example with all available fields

#### Added
- `default_format` config field (`yaml:"default_format"`) - sets a default credential output format
  from the config file, skipping the format selection step in the TUI

- `pkg/update` package for CLI self-update functionality
  - `CheckLatestVersion()` - fetches latest GitHub release, compares semver
  - `DownloadAndReplace()` - downloads release archive, verifies SHA256 checksum against `checksums.sha256`,
     extracts binary from `.tar.gz` (Linux/macOS) or `.zip` (Windows), performs atomic binary replacement
  - `CompareVersions()` - semver comparison
  - `BuildAssetName()` - constructs GoReleaser-compatible asset filenames

-  The validation seperatd to the `internal/validators` package - extracted named validation functions (email, ARN, URL, region, etc.)

- Update check on startup
  - Banner at the top of the right content panel when a new version is available
  - `--no-update-check` flag, `J2A_NO_UPDATE_CHECK` env var, and `no_update_check` YAML config field to disable
  - `--update` flag for standalone self-update mode (downloads latest `jc2aws` release from GitHub)
  - `NoUpdateCheck` field in `internal/config.Config` struct (`yaml:"no_update_check"`)
  - `DefaultFormat` field in `internal/config.Config` struct (`yaml:"default_format"`) - allows setting a
   default credential output format from the config file

- `tui_done_action` config field (`yaml:"tui_done_action"`) - controls TUI behavior after writing
  file-based credentials (`cli`, `env` formats). Options: `"exit"` (default, immediate close),
  `"menu"` (show Run again/Quit choice), `"wait"` (press any key to continue). Config-file only, no CLI flag.
  - `TUIDoneAction` field in `internal/config.Config` struct (`yaml:"tui_done_action"`) - allows configuring
     TUI behavior after writing file-based credentials

#### Deprecated
- `session_timeout` config field is still accepted as an alias
  for `session_duration` in account configs. If `session_duration` is not set but `session_timeout` is,
  the value is migrated automatically. `session_timeout` will be removed in a future release.

## [3.0.1] - 2026-03-17

#### Fixed
- HTTP client configuration in JumpCloud auth flow
- Various small fixes and code cleanup

## [3.0.0] - 2026-03-16

#### Changed
- Updated Go to v1.26.0
- Reviewed and cleaned up validators
- Removed unused getters from config package
- Added error handling when home directory cannot be determined
- Removed unused error variables

#### Fixed
- Session duration handling
- Fixed function documentation (`ToAwsConfig`, `ToAwsSamlOutput`)
- Fixed `GetCredentials` return values
- Fixed `NewConfig` error text
- Fixed `GetHTMLInputValue` sanitizer
- Fixed `GetSaml` return value and context library usage
- Fixed HTTP client handling
- Fixed config file loading
- Fixed duration setting
- Fixed error handlers
- Fixed `~/.aws` directory creation when it does not exist
- Small fix and refactoring (#16)

## [2.2.0] - 2025-10-13

#### Added
- Docker image builds on `ghcr.io/yousysadmin/jc2aws` (multi-arch: amd64, arm64)
- GoReleaser v2 configuration (`.goreleaser.yml`)
- LICENSE file
- Install script (`scripts/install.sh`)

#### Fixed
- Dockerfile fixes

## [2.1.0] - 2025-10-11

#### Fixed
- AWS region handling (#14)
- Test fixes

#### Changed
- Updated Go to v1.25.2
- Updated dependencies

## [2.0.0] - 2025-09-07

#### Changed
- Major CLI code refactoring (#13)
- Updated Go to v1.25.0

#### Fixed
- TOTP generator (#12)
- Code formatting

## [1.1.0] - 2025-09-06

#### Fixed
- TOTP generator (#12)

#### Changed
- Updated Go to v1.25.0
- Code formatting fixes

## [1.0.2] - 2025-07-19

#### Fixed
- GoReleaser Go version configuration

## [1.0.1] - 2025-03-10

#### Changed
- Merge #8

## [1.0.0] - 2024-12-25

Initial release.

[Unreleased]: https://github.com/YouSysAdmin/jc2aws/compare/v4.3.0...HEAD
[4.3.0]: https://github.com/YouSysAdmin/jc2aws/compare/v4.2.0...v4.3.0
[4.2.0]: https://github.com/YouSysAdmin/jc2aws/compare/v4.1.0...v4.2.0
[4.1.0]: https://github.com/YouSysAdmin/jc2aws/compare/v4.0.1...v4.1.0
[4.0.1]: https://github.com/YouSysAdmin/jc2aws/compare/v4.0.0...v4.0.1
[4.0.0]: https://github.com/YouSysAdmin/jc2aws/compare/v3.0.1...v4.0.0
[3.0.1]: https://github.com/YouSysAdmin/jc2aws/compare/v3.0.0...v3.0.1
[3.0.0]: https://github.com/YouSysAdmin/jc2aws/compare/v2.2.0...v3.0.0
[2.2.0]: https://github.com/YouSysAdmin/jc2aws/compare/v2.1.0...v2.2.0
[2.1.0]: https://github.com/YouSysAdmin/jc2aws/compare/v2.0.0...v2.1.0
[2.0.0]: https://github.com/YouSysAdmin/jc2aws/compare/v1.1.0...v2.0.0
[1.1.0]: https://github.com/YouSysAdmin/jc2aws/compare/v1.0.2...v1.1.0
[1.0.2]: https://github.com/YouSysAdmin/jc2aws/compare/v1.0.1...v1.0.2
[1.0.1]: https://github.com/YouSysAdmin/jc2aws/compare/v1.0.0...v1.0.1
[1.0.0]: https://github.com/YouSysAdmin/jc2aws/releases/tag/v1.0.0
