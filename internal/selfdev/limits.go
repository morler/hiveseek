// Package selfdev records the fixed limits for future self-development gates.
// M0 does not register commands, grant capabilities, or execute candidates.
package selfdev

import "time"

const (
	// VerificationTimeout bounds the entire build and verification operation,
	// not each individual step. M0 measured 18.283554s with a warm Go cache;
	// retain the 15-minute allowance for slower machines and cold builds.
	VerificationTimeout = 15 * time.Minute

	// ReadyTimeout bounds candidate startup. M0 fresh-process app-server ready
	// samples were 0.014532s, 0.014713s, and 0.013537s. They are a startup proxy,
	// not proof of the future activation handshake. Retain the 30-second limit.
	ReadyTimeout = 30 * time.Second
)

// Maintainers adjust these constants by reviewed code changes, accompanied by
// new measurements in docs/self-evolution/m0-baseline.md. No runtime override
// or model-supplied duration is part of this contract.
