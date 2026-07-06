package extractor

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var (
	KEY_SEGMENT        = regexp.MustCompile(`[?&]_t=([^&\s]+)`)
	ENCRYPTED_PLAYLIST = regexp.MustCompile(`[?&]_c=\d+`)
	ENCRYPTED_SEGMENT  = regexp.MustCompile(`(?i)/hls/([0-9a-f]{24})\.ts`)
	INF_TAG            = regexp.MustCompile(`^#EXTINF:`)
	ENDLIST_TAG        = regexp.MustCompile(`^#EXT-X-ENDLIST`)
	KEY_TAG            = regexp.MustCompile(`^#EXT-X-KEY`)
)

// Envelope represents the structural metadata used during decryption.
type Envelope struct {
	CN  string `json:"cn"`
	SK  string `json:"sk"`
	TS  string `json:"ts"`
	UID string `json:"uid"`
}

func extractEnvelope(rawHeaders http.Header) Envelope {

	envelopeHeader := rawHeaders.Get("X-Envelope")
	if envelopeHeader != "" {
		// Mimics the try/catch behavior; if decoding fails, it falls through to legacy headers
		if env, err := parseEnvelope(envelopeHeader); err == nil {
			return env
		}
	}

	uidRaw := rawHeaders.Get("X-Proxy-Digest")
	if uidRaw == "" {
		uidRaw = "anon"
	}
	uid, err := url.QueryUnescape(uidRaw)
	if err != nil {
		uid = uidRaw // Fallback to raw string if URI decoding fails
	}

	ts := rawHeaders.Get("X-Request-Trace")
	if ts == "" {
		ts = "0"
	}

	return Envelope{
		CN:  rawHeaders.Get("X-Edge-Tag"),
		SK:  rawHeaders.Get("X-Cache-Node"),
		TS:  ts,
		UID: uid,
	}
}

func decryptPlaylist(raw []byte, envelope *Envelope, token string, originHost string) ([]byte, error) {
	rawPlaylist := string(raw)

	var cn, sk, ts, uid string
	if envelope != nil {
		cn = envelope.CN
		sk = envelope.SK
		ts = envelope.TS
		uid = envelope.UID
	}
	if ts == "" {
		ts = "0"
	}
	if uid == "" {
		uid = "anon"
	}

	lines := strings.Split(rawPlaylist, "\n")
	encryptedPlaylist := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || trimmed == "" {
			continue
		}

		if ENCRYPTED_PLAYLIST.MatchString(line) {
			encryptedPlaylist = true
		}
		break
	}

	if !encryptedPlaylist || cn == "" || sk == "" {
		return raw, nil
	}

	var metadataLines []string
	var encryptedTokens []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || trimmed == "" {
			if !INF_TAG.MatchString(line) && !ENDLIST_TAG.MatchString(line) && !KEY_TAG.MatchString(line) {
				metadataLines = append(metadataLines, line)
			}
			continue
		}

		match := KEY_SEGMENT.FindStringSubmatch(line)
		if match != nil {
			encryptedTokens = append(encryptedTokens, match[1])
		}
	}

	bundledCiphertext := strings.Join(encryptedTokens, "")
	if bundledCiphertext == "" {
		return raw, nil
	}

	shuffledCiphertext := preprocessCiphertext(bundledCiphertext, sk)

	decryptedBody, err := decryptPlaylistBody(shuffledCiphertext, cn, sk, uid, ts)
	if err != nil {
		return nil, err
	}

	bodyLines := strings.Split(decryptedBody, "\n")
	var absoluteBodyLines []string
	for _, line := range bodyLines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || trimmed == "" {
			absoluteBodyLines = append(absoluteBodyLines, line)
			continue
		}

		if strings.HasPrefix(line, "/") {
			absoluteBodyLines = append(absoluteBodyLines, originHost+line)
		} else {
			absoluteBodyLines = append(absoluteBodyLines, line)
		}
	}

	var decryptedLines []string
	for _, line := range absoluteBodyLines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || trimmed == "" {
			decryptedLines = append(decryptedLines, line)
			continue
		}

		match := ENCRYPTED_SEGMENT.FindStringSubmatch(line)
		if match == nil {

			decryptedLines = append(decryptedLines, line)
			continue
		}

		decryptedURL, err := decryptSegmentURL(line, token)
		if err != nil {
			return nil, err
		}
		decryptedLines = append(decryptedLines, decryptedURL)
	}

	finalPlaylist := strings.Join(metadataLines, "\n") + "\n" + strings.Join(decryptedLines, "\n")
	return []byte(finalPlaylist), nil
}

func decryptSegmentURL(inputURL string, token string) (string, error) {
	masterKey := extractSessionID(token)
	if masterKey == "" {
		return "", fmt.Errorf("unable to derive master key from token")
	}

	u, err := url.Parse(inputURL)
	if err != nil {
		return "", err
	}

	match := ENCRYPTED_SEGMENT.FindStringSubmatch(u.Path)
	if match == nil {
		return "", fmt.Errorf("unable to extract file id from %s", inputURL)
	}
	fileID := match[1]

	counterStr := u.Query().Get("i")
	counterValue, _ := strconv.ParseInt(counterStr, 10, 64)

	encryptedData := u.Query().Get("e")
	if encryptedData == "" {
		return "", fmt.Errorf("missing e parameter in %s", inputURL)
	}

	mac := hmac.New(sha256.New, []byte(masterKey))
	mac.Write([]byte(fmt.Sprintf("url-cipher|%s", fileID)))
	aesMaterial := mac.Sum(nil)

	block, err := aes.NewCipher(aesMaterial)
	if err != nil {
		return "", err
	}

	counter := make([]byte, 16)
	counter[12] = byte((counterValue >> 24) & 0xff)
	counter[13] = byte((counterValue >> 16) & 0xff)
	counter[14] = byte((counterValue >> 8) & 0xff)
	counter[15] = byte(counterValue & 0xff)

	stream := cipher.NewCTR(block, counter)

	encryptedBytes, err := base64UrlToBytes(encryptedData)
	if err != nil {
		return "", err
	}

	plaintext := make([]byte, len(encryptedBytes))
	stream.XORKeyStream(plaintext, encryptedBytes)

	return string(plaintext), nil
}

func decryptPlaylistBody(ciphertext, cn, sk, uid, ts string) (string, error) {
	cnBytes, err := base64UrlToBytes(cn)
	if err != nil {
		return "", err
	}

	mac := hmac.New(sha256.New, cnBytes)
	fmt.Fprintf(mac, "%s:%s:%s:0", uid, ts, sk)
	signature := mac.Sum(nil)

	block, err := aes.NewCipher(signature)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	if len(cnBytes) < 12 {
		return "", fmt.Errorf("cnBytes configuration too short for IV extraction")
	}
	iv := cnBytes[:12]

	cipherBytes, err := base64UrlToBytes(ciphertext)
	if err != nil {
		return "", err
	}

	decrypted, err := gcm.Open(nil, iv, cipherBytes, nil)
	if err != nil {
		return "", err
	}

	unwrapped := permuteAndXor(decrypted, sk, ts)
	return string(unwrapped), nil
}

func extractSessionID(token string) string {
	if token == "" {
		return ""
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}

	payloadStr := parts[1]
	payloadStr = strings.ReplaceAll(payloadStr, "-", "+")
	payloadStr = strings.ReplaceAll(payloadStr, "_", "/")
	switch len(payloadStr) % 4 {
	case 2:
		payloadStr += "=="
	case 3:
		payloadStr += "="
	}

	decoded, err := base64.StdEncoding.DecodeString(payloadStr)
	if err != nil {
		return ""
	}

	var jsonMap map[string]interface{}
	if err := json.Unmarshal(decoded, &jsonMap); err != nil {
		return ""
	}

	jti, ok := jsonMap["jti"].(string)
	if !ok {
		return ""
	}

	jtiRunes := []rune(jti)
	var result strings.Builder
	for i := 1; i < len(jtiRunes); i += 2 {
		result.WriteRune(jtiRunes[i])
	}

	return result.String()
}

func preprocessCiphertext(payload string, key string) string {
	subKey := key

	if len(subKey) > 8 {
		subKey = subKey[:8]
	}
	seed := parseHexPrefix(subKey)

	chars := []rune(payload)
	type swap struct {
		a, b int
	}
	swaps := make([]swap, 0, len(chars))

	for i := len(chars) - 1; i > 0; i-- {
		seed = seed*1664525 + 1013904223
		swaps = append(swaps, swap{a: i, b: int(seed % uint32(i+1))})
	}

	for i := len(swaps) - 1; i >= 0; i-- {
		sw := swaps[i]
		chars[sw.a], chars[sw.b] = chars[sw.b], chars[sw.a]
	}

	return string(chars)
}

func permuteAndXor(buffer []byte, permKey string, salt string) []byte {
	output := make([]byte, len(buffer))
	if len(buffer) == 0 {
		return output
	}

	rng := createSeededRng(permKey + "|" + salt)
	permutation := createPermutation(rng, len(buffer))

	var rand32 uint32 = 0
	for i := 0; i < len(buffer); i++ {
		if (i & 3) == 0 {
			rand32 = rng()
		}

		keyByte := byte((rand32 >> (8 * (i & 3))) & 0xff)
		output[permutation[i]] = buffer[i] ^ keyByte
	}

	return output
}

func createPermutation(rng func() uint32, n int) []uint32 {
	permutation := make([]uint32, n)
	for i := 0; i < n; i++ {
		permutation[i] = uint32(i)
	}

	for i := n - 1; i > 0; i-- {
		j := int(rng() % uint32(i+1))
		permutation[i], permutation[j] = permutation[j], permutation[i]
	}
	return permutation
}

func createSeededRng(seed string) func() uint32 {
	var state uint32 = 0x811c9dc5

	for i := 0; i < len(seed); i++ {
		state ^= uint32(seed[i]) & 0xff
		state = state * 0x01000193
	}

	if state == 0 {
		state = 1
	}

	return func() uint32 {
		state ^= state << 13
		state ^= state >> 17
		state ^= state << 5
		return state
	}
}

func base64UrlToBytes(text string) ([]byte, error) {
	normalized := strings.ReplaceAll(text, "-", "+")
	normalized = strings.ReplaceAll(normalized, "_", "/")
	switch len(normalized) % 4 {
	case 2:
		normalized += "=="
	case 3:
		normalized += "="
	}
	return base64.StdEncoding.DecodeString(normalized)
}

func parseEnvelope(raw string) (Envelope, error) {
	bytes, err := base64UrlToBytes(raw)
	if err != nil {
		return Envelope{}, err
	}

	if len(bytes) < 11 {
		return Envelope{}, fmt.Errorf("envelope too short")
	}

	if bytes[0] != 0x55 || bytes[1] != 0x53 || bytes[2] != 0x44 || bytes[3] != 0x4b {
		return Envelope{}, fmt.Errorf("invalid envelope magic")
	}

	if bytes[4] != 1 {
		return Envelope{}, fmt.Errorf("unsupported envelope version %d", bytes[4])
	}

	payloadLength := (int(bytes[5]) << 8) | int(bytes[6])

	if len(bytes) < 7+payloadLength+4 {
		return Envelope{}, fmt.Errorf("truncated envelope")
	}

	payload := bytes[7 : 7+payloadLength]

	storedCrc := (uint32(bytes[7+payloadLength]) << 24) |
		(uint32(bytes[8+payloadLength]) << 16) |
		(uint32(bytes[9+payloadLength]) << 8) |
		uint32(bytes[10+payloadLength])

	actualCrc := crc32.ChecksumIEEE(payload)

	if storedCrc != actualCrc {
		return Envelope{}, fmt.Errorf("CRC mismatch")
	}

	var env Envelope
	if err := json.Unmarshal(payload, &env); err != nil {
		return Envelope{}, err
	}

	return env, nil
}

func parseHexPrefix(s string) uint32 {
	end := 0
	for end < len(s) && isHexDigit(s[end]) {
		end++
	}
	if end == 0 {
		return 0
	}
	v, _ := strconv.ParseUint(s[:end], 16, 32)
	return uint32(v)
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') ||
		(c >= 'a' && c <= 'f') ||
		(c >= 'A' && c <= 'F')
}
