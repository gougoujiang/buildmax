package util

import (
	"math/big"

	"github.com/google/uuid"
)

// Prefix constants for prefixed IDs. Use with NewPrefixedID.
// Semantics: u=user, w=workspace, a=agent, c=task, r=task run, ar=artifact, f=artifact item, cv=conversation, cm=conversation message, whk=workspace webhook key.
// Session IDs are internal (not user-facing) and use UUID; see entity.CreateChat.
const (
	PrefixUser                = "u"
	PrefixWorkspace           = "w"
	PrefixAgent               = "a"
	PrefixChat                = "c"
	PrefixTaskRun             = "r"
	PrefixArtifact            = "ar"
	PrefixArtifactItem        = "f"
	PrefixConversation        = "cv"
	PrefixConversationMessage = "cm"
	PrefixWebhookKey          = "whk"
)

const (
	idBodyLen       = 20
	idBase36        = 36
	idAlphabetLower = "0123456789abcdefghijklmnopqrstuvwxyz"
)

var (
	idBig36   = big.NewInt(idBase36)
	idMaxBody = new(big.Int).Exp(big.NewInt(idBase36), big.NewInt(idBodyLen), nil)
)

// NewPrefixedID returns a string of the form "<prefix>_<body>", where body is 20
// characters from [a-z0-9] (lowercase base36), derived from 128 bits of
// crypto-random entropy. Ordering is by created_at; IDs are not time-ordered.
func NewPrefixedID(prefix string) string {
	u := uuid.New()
	n := new(big.Int).SetBytes(u[:])
	n.Mod(n, idMaxBody)
	return prefix + "_" + encodeBase36Lower(n, idBodyLen)
}

// encodeBase36Lower encodes a non-negative big.Int to a base36 string (0-9, a-z),
// zero-padded to length characters.
func encodeBase36Lower(n *big.Int, length int) string {
	if n.Sign() < 0 {
		n = new(big.Int).Set(n)
		n.Neg(n)
	}
	buf := make([]byte, length)
	for i := range buf {
		buf[i] = idAlphabetLower[0]
	}
	mod := new(big.Int)
	pos := length - 1
	for n.Sign() > 0 && pos >= 0 {
		n.DivMod(n, idBig36, mod)
		buf[pos] = idAlphabetLower[mod.Int64()]
		pos--
	}
	return string(buf)
}
