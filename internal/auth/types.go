package auth

import (
	authcodex "cliro-go/internal/auth/codex"
	authkiro "cliro-go/internal/auth/kiro"
	syncauth "cliro-go/internal/sync/authtoken"
)

type SessionStatus = string

const (
	SessionPending SessionStatus = "pending"
	SessionSuccess SessionStatus = "success"
	SessionError   SessionStatus = "error"
)

type CodexAuthStart = authcodex.AuthStart
type CodexAuthSessionView = authcodex.AuthSessionView

type KiroAuthStart = authkiro.AuthStart
type KiroAuthSessionView = authkiro.AuthSessionView

type KiloAuthSyncResult = syncauth.KiloAuthSyncResult
type OpencodeAuthSyncResult = syncauth.OpencodeAuthSyncResult
type CodexAuthSyncResult = syncauth.CodexAuthSyncResult
