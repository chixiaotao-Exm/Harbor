package fingerprint

import "encoding/base64"

func decoder(s string) ([]byte, error) { return base64.StdEncoding.DecodeString(s) }
