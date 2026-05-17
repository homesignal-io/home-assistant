package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

func MaterialHash(material map[string]string) string {
	keys := make([]string, 0, len(material))
	for key := range material {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var builder strings.Builder
	for _, key := range keys {
		builder.WriteString(key)
		builder.WriteByte('=')
		builder.WriteString(material[key])
		builder.WriteByte('\n')
	}

	sum := sha256.Sum256([]byte(builder.String()))
	return hex.EncodeToString(sum[:])
}
