# Requirements Document

## Introduction

This feature hardens app startup so corrupted local config data does not crash CLIro-Go, and refines single-instance behavior so launching the app a second time restores the existing window instead of feeling like a failed launch. It also upgrades backend tests around config load/recovery to reduce regressions.

## Requirements

### Requirement 1: Startup Resilience for Corrupted Config Files

**User Story:** As a CLIro-Go user, I want the app to keep launching even when local config files are corrupted, so that I can still access the UI and recover quickly.

#### Acceptance Criteria

1.1 WHEN startup reads `config.json` AND the JSON is malformed THEN the system SHALL avoid panic and continue startup using default app settings.
1.2 WHEN startup reads `accounts.json` AND the JSON is malformed THEN the system SHALL avoid panic and continue startup with an empty account list.
1.3 WHEN startup reads `stats.json` AND the JSON is malformed THEN the system SHALL avoid panic and continue startup with zeroed stats values.
1.4 IF a corrupted config file is detected THEN the system SHALL preserve the original bytes in a timestamped backup file before writing recovered content.
1.5 WHEN recovery succeeds THEN the system SHALL write a valid canonical JSON document back to the original file path.

### Requirement 2: Strict Data Format Policy (No Backward Compatibility Parsing)

**User Story:** As a maintainer, I want strict config format handling, so that the codebase does not carry long-term backward parsing branches.

#### Acceptance Criteria

2.1 WHEN `accounts.json` contains a non-array root object (including legacy envelopes like `{ "accounts": [...] }`) THEN the system SHALL treat the file as invalid/corrupt input.
2.2 WHEN invalid/corrupt input is detected THEN the system SHALL recover through corruption handling, not through legacy format deserialization.
2.3 WHERE config loaders parse on startup, the system SHALL keep a single canonical read format per file type.

### Requirement 3: Actionable Error Handling and Logging

**User Story:** As an operator, I want clear runtime signals when recovery happened, so that I can diagnose and fix local environment issues.

#### Acceptance Criteria

3.1 WHEN any config file is recovered from corruption THEN the system SHALL emit an explicit warning/error log entry with file path and backup path.
3.2 IF recovery write-back fails THEN the system SHALL surface a descriptive error and SHALL still avoid abrupt panic in startup flow.
3.3 WHEN startup detects one or more recovered configuration files THEN the system SHALL show a visible prompt/modal after initial UI mount.
3.4 WHEN the configuration error modal is shown THEN the system SHALL include affected file path(s), backup path(s), and a concise recovery message.
3.5 IF multiple configuration files are recovered in the same startup THEN the system SHALL aggregate them into one modal with a list of issues.
3.6 WHEN the user dismisses the configuration error modal THEN the system SHALL continue normal app flow without requiring restart.

### Requirement 4: Reliable Automated Tests for Config Corruption Paths

**User Story:** As a developer, I want reliable tests for startup config handling, so that corrupt-file regressions are caught before release.

#### Acceptance Criteria

4.1 WHEN running config tests THEN malformed JSON cases for settings/accounts/stats SHALL be covered with deterministic assertions.
4.2 WHEN malformed JSON is provided THEN tests SHALL assert no startup panic path is triggered through config manager initialization.
4.3 WHEN corruption recovery executes THEN tests SHALL assert backup file creation and canonical rewritten output.
4.4 WHERE tests validate account loading, the suite SHALL verify strict non-array rejection behavior without backward parser branches.

### Requirement 5: Single-Instance Foreground Behavior on Second Launch

**User Story:** As a desktop user, I want a second app launch attempt to focus the existing window, so that the app feels responsive and I do not think it failed to open.

#### Acceptance Criteria

5.1 IF an app instance is already running AND the user launches the app again THEN the system SHALL keep a single process window (no additional window).
5.2 WHEN a second-launch event is received by the running instance THEN the system SHALL restore the main window from minimized/hidden state and bring it to the foreground.
5.3 WHEN the running instance handles second launch THEN the system SHALL emit a UI event that triggers a visible in-app notice (toast/modal) indicating the existing app was reopened.
5.4 IF second-launch metadata is available (args/working directory) THEN the system SHALL include that metadata in diagnostic logging/event payload.

### Requirement 6: Banned Account Detection

**User Story:** As an operator, I want accounts with banned/invalidated credentials to be detected explicitly, so that I can stop retrying broken accounts and understand why they no longer work.

#### Acceptance Criteria

6.1 WHEN a provider returns a token-invalidated/auth-revoked signal for an account THEN the system SHALL classify that account as banned rather than a transient auth error.
6.2 IF an account is classified as banned THEN the system SHALL persist banned status in local account state.
6.3 WHEN an account is marked as banned THEN the system SHALL exclude it from normal account rotation/selection.
6.4 WHEN an account is marked as banned THEN the system SHALL expose a clear UI-visible status and reason indicating token invalidated / suspected banned account.
6.5 WHEN banned detection occurs THEN the system SHALL emit a log entry describing the provider, account, and detection reason.
6.6 WHERE automated tests cover auth/account status transitions, the suite SHALL include banned-detection scenarios for token invalidated responses.

### Requirement 7: Request Routing Observability and Usage Logging

**User Story:** As an operator, I want every routed request and response to be logged clearly, so that I can understand which account/provider handled traffic and how much usage it consumed.

#### Acceptance Criteria

7.1 WHEN a request is routed through an account THEN the system SHALL emit a structured log entry containing request identifier, provider, account identifier, selected model, and route family.
7.2 WHEN a request completes successfully THEN the system SHALL emit a structured log entry containing prompt tokens, completion tokens, total tokens, latency, and finish status when available.
7.3 WHEN a request fails or retries across accounts THEN the system SHALL emit log entries for each attempt including provider/account used and failure reason.
7.4 WHERE sensitive values are present, the system SHALL avoid logging raw tokens, authorization headers, or secret credentials.
7.5 WHEN account-level usage counters are updated THEN the logged usage details SHALL remain consistent with persisted account and global stats.

### Requirement 8: API Router Compatibility and Security Correctness

**User Story:** As an API router operator, I want the proxy endpoint shape and security behavior to be correct by default, so that client integrations work predictably and safely.

#### Acceptance Criteria

8.1 WHERE the local proxy base URL is shown in UI or docs, the system SHALL default to `http://127.0.0.1:<port>` without appending `/v1`.
8.2 IF a user or client supplies an upstream/base path override containing `/v1` THEN the system SHALL avoid producing doubled paths like `/v1/v1`.
8.3 WHEN OpenAI-compatible routes are served THEN the router SHALL support `/responses` under the `v1` route family in addition to currently supported endpoints.
8.4 WHEN proxy requests are processed THEN the router SHALL validate and normalize relevant security-sensitive headers consistently.
8.5 WHEN the security/header handling rejects a request THEN the system SHALL return a clear protocol-appropriate error response and log the rejection reason.
8.6 WHERE automated tests cover router behavior, the suite SHALL include base-path normalization, `/responses` routing, and security/header validation scenarios.
