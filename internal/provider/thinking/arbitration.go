package thinking

import (
	contract "cliro-go/internal/contract"
	"strings"
)

type Source string

const (
	SourceNone   Source = "none"
	SourceNative Source = "native"
	SourceParsed Source = "parsed"
	SourceForced Source = "forced"
)

type Candidate struct {
	Thinking  string
	Signature string
}

type Inputs struct {
	Request      contract.ThinkingConfig
	ForceAllowed bool
	Native       Candidate
	Parsed       Candidate
	Forced       Candidate
}

type Selection struct {
	Source    Source
	Thinking  string
	Signature string
}

func Select(inputs Inputs) Selection {
	if candidate, ok := selectCandidate(inputs.Native); ok {
		return Selection{Source: SourceNative, Thinking: candidate.Thinking, Signature: candidate.Signature}
	}
	if candidate, ok := selectCandidate(inputs.Parsed); ok {
		return Selection{Source: SourceParsed, Thinking: candidate.Thinking, Signature: candidate.Signature}
	}
	if inputs.Request.Requested && inputs.ForceAllowed {
		if candidate, ok := selectCandidate(inputs.Forced); ok {
			return Selection{Source: SourceForced, Thinking: candidate.Thinking, Signature: candidate.Signature}
		}
	}
	return Selection{Source: SourceNone}
}

func ForceEligible(request contract.ThinkingConfig, forceConfigured bool) bool {
	if !request.Requested || !forceConfigured {
		return false
	}
	return request.Mode == contract.ThinkingModeForce || request.Mode == contract.ThinkingModeAuto
}

type Arbiter struct {
	selected Source
}

func (a *Arbiter) Allow(source Source) bool {
	if normalizeSource(source) == SourceNone {
		return false
	}
	if normalizeSource(a.selected) == SourceNone {
		a.selected = source
		return true
	}
	return a.selected == source
}

func (a *Arbiter) Selected() Source {
	return normalizeSource(a.selected)
}

func selectCandidate(candidate Candidate) (Candidate, bool) {
	if strings.TrimSpace(candidate.Thinking) == "" {
		return Candidate{}, false
	}
	return candidate, true
}

func normalizeSource(source Source) Source {
	if source == "" {
		return SourceNone
	}
	return source
}
