# Requirements Document

## Introduction

This feature fixes provider availability regressions and protocol parity gaps that currently cause healthy accounts to leave the rotation too aggressively, produce repeated `provider_unavailable` errors, and send incompatible response shapes across protocol boundaries. The scope covers Kiro and Codex account lifecycle handling, Kiro runtime parity with the reference implementation, and OpenAI/Anthropic response compatibility corrections.

## Requirements

### Requirement 1: Preserve Provider Availability During Recoverable Failures

**User Story:** As an operator, I want recoverable provider failures to avoid draining the account pool, so that the proxy does not collapse into repeated `no available ... accounts` errors.

#### Acceptance Criteria

1.1 WHEN a provider request fails with a recoverable auth, retryable transport, or short-lived upstream error THEN the system SHALL avoid immediately removing all matching accounts from normal rotation.
1.2 WHEN Kiro or Codex returns a recoverable failure for one account THEN the system SHALL classify the failure into retry, short cooldown, long cooldown, banned, or immediate request error instead of using one coarse path.
1.3 IF an account is placed into cooldown for a recoverable failure THEN the system SHALL preserve at least one remaining available account when other healthy accounts still exist.
1.4 WHEN all accounts for a provider become temporarily unavailable THEN the system SHALL expose the exact reason breakdown in logs instead of only returning a generic pool-empty condition.
1.5 WHERE provider availability is calculated, the system SHALL distinguish temporary cooldown from durable disabled/banned state.

### Requirement 2: Kiro Runtime Request Parity With Reference Behavior

**User Story:** As a maintainer, I want Kiro runtime requests to match the proven reference behavior, so that valid Kiro accounts do not fail due to header, endpoint, or payload mismatches.

#### Acceptance Criteria

2.1 WHEN sending a request to the primary Amazon Q Kiro endpoint THEN the system SHALL omit `X-Amz-Target` if the reference behavior omits it.
2.2 WHEN sending a request to the fallback CodeWhisperer Kiro endpoint THEN the system SHALL send the correct `X-Amz-Target` and Kiro-specific headers used by the reference behavior.
2.3 WHEN Kiro request construction depends on account/profile metadata THEN the system SHALL inject `profileArn` only in the cases where it is required and SHALL suppress it in the auth modes where the reference behavior suppresses it.
2.4 WHEN Kiro receives a token-related `401` or retryable `403` THEN the system SHALL attempt the same class of refresh/retry behavior as the reference before marking the account unavailable.
2.5 WHEN the primary Kiro endpoint fails with a retryable transport error THEN the system SHALL attempt the reference fallback behavior before failing the request.

### Requirement 3: Codex and Kiro Cooldown/Disable Policy Hardening

**User Story:** As an operator, I want cooldown and disable rules to reflect actual provider state, so that healthy accounts remain usable and broken accounts are isolated accurately.

#### Acceptance Criteria

3.1 WHEN a provider returns quota exhaustion or rate limiting THEN the system SHALL use a quota-specific temporary cooldown policy rather than disabling the account.
3.2 WHEN a provider returns a suspended, revoked, or durable auth failure signal THEN the system SHALL use a durable unavailable state distinct from transient cooldown.
3.3 WHEN a request fails because of request-shape parity, header mismatch, or unsupported route translation THEN the system SHALL not mark the account as banned.
3.4 WHEN Codex or Kiro refresh succeeds after a token-related failure THEN the system SHALL restore the account to healthy rotation state automatically.
3.5 WHERE pool selection counts availability, the system SHALL report temporary cooldown, durable disabled, and banned counts separately for diagnostics.

### Requirement 4: OpenAI Responses to Anthropic Compatibility Fixes

**User Story:** As an API user, I want translated requests and responses to respect protocol-specific content type rules, so that cross-protocol routing does not fail with invalid part-type errors.

#### Acceptance Criteria

4.1 WHEN translating OpenAI Responses-style content into Anthropic-compatible message payloads THEN the system SHALL not emit content part types that Anthropic rejects in that direction.
4.2 WHEN assistant-side text is translated for Anthropic responses or follow-up requests THEN the system SHALL use the correct allowed text part type for that role and phase.
4.3 IF a translated request would otherwise contain unsupported values such as `input_text` in an Anthropic-incompatible position THEN the system SHALL normalize it before sending upstream.
4.4 WHEN a protocol translation fix is applied THEN the system SHALL preserve user-visible text/tool semantics and SHALL not silently drop valid content.

### Requirement 5: Session and Continuation Fidelity for Kiro Requests

**User Story:** As a user of long-running Kiro conversations, I want continuation and conversation metadata to survive routing, so that follow-up turns do not break due to missing runtime context.

#### Acceptance Criteria

5.1 WHEN the incoming request already contains usable conversation or continuation metadata THEN the system SHALL preserve and forward that metadata through the Kiro runtime request path.
5.2 WHEN Kiro tool or agent follow-up turns are routed THEN the system SHALL keep the request history and continuation identifiers in a form compatible with the reference behavior.
5.3 IF the request lacks reusable continuation metadata THEN the system SHALL generate safe defaults without breaking normal one-shot requests.

### Requirement 6: Diagnostics and Test Coverage for Availability Regressions

**User Story:** As a developer, I want deterministic tests and clear logs around provider availability state, so that pool-drain and parity regressions are caught before release.

#### Acceptance Criteria

6.1 WHEN automated tests run THEN the suite SHALL cover Kiro primary/fallback request construction parity, including header and `profileArn` behavior.
6.2 WHEN automated tests run THEN the suite SHALL cover Codex and Kiro transient failure classification versus durable unavailable classification.
6.3 WHEN automated tests run THEN the suite SHALL cover the `no available codex accounts` and `no available kiro accounts` scenarios to ensure they only occur after all accounts are truly unavailable.
6.4 WHEN automated tests run THEN the suite SHALL cover the Anthropic/OpenAI content-type normalization that prevents errors like `Invalid value: 'input_text'. Supported values are: 'output_text' and 'refusal'.`
6.5 WHEN provider availability changes because of retries, cooldowns, or durable failures THEN the system SHALL emit logs that identify provider, account, classification reason, and resulting availability state.
