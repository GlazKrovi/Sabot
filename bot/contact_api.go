package bot

import (
	"bytes"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

func contactApi[T any](httpMethod string, apiPath string, bodyData any, printInfo bool) (T, error) {
	var result T

	// 1. Load your Private Key
	privateKeyPem, err := os.ReadFile("private.pem")
	if err != nil {
		return result, err
	}

	block, _ := pem.Decode(privateKeyPem)
	if block == nil {
		return result, fmt.Errorf("invalid PEM file")
	}

	privateKeyAny, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return result, err
	}

	privateKey, ok := privateKeyAny.(ed25519.PrivateKey)
	if !ok {
		return result, fmt.Errorf("not an Ed25519 private key")
	}

	// 2. Extract path from URL for signature
	parsedURL, err := url.Parse(apiPath)
	if err != nil {
		return result, err
	}
	// Path must start from /api according to Revolut docs
	pathForSignature := parsedURL.Path
	// Query string WITHOUT the '?' according to Revolut docs
	if parsedURL.RawQuery != "" {
		pathForSignature += parsedURL.RawQuery
	}

	// 3. Prepare message
	// Use milliseconds to match JavaScript's `Date.now()`
	// We remove 3s as revolit api as suspect clock skew
	timestamp := fmt.Sprintf("%d", time.Now().UnixMilli()-3000)

	var bodyStr string
	if bodyData == nil {
		bodyStr = ""
	} else if m, ok := bodyData.(map[string]string); ok && len(m) == 0 {
		// Empty map - no body
		bodyStr = ""
	} else {
		// Marshal to compact JSON (works for structs, maps, etc.)
		bodyBytes, err := json.Marshal(bodyData)
		if err != nil {
			return result, err
		}
		bodyStr = string(bodyBytes)
	}

	message := timestamp + httpMethod + pathForSignature + bodyStr
	if printInfo {
		fmt.Printf("Message to sign: '%s'\n", message)
		fmt.Printf("bodyData: %+v\n", bodyData)
		fmt.Printf("bodyStr: %s\n", bodyStr)
	}

	// 4. Sign
	signatureBytes := ed25519.Sign(privateKey, []byte(message))
	signature := base64.StdEncoding.EncodeToString(signatureBytes)

	// Local verification to help debug signature mismatches
	var pub ed25519.PublicKey
	if len(privateKey) == ed25519.SeedSize {
		full := ed25519.NewKeyFromSeed(privateKey)
		pub = full.Public().(ed25519.PublicKey)
	} else if len(privateKey) >= ed25519.PrivateKeySize {
		pub = ed25519.PublicKey(privateKey[32:])
	}

	if printInfo {
		if pub != nil {
			if ok := ed25519.Verify(pub, []byte(message), signatureBytes); !ok {
				fmt.Println("Local signature verification: FAILED")
			} else {
				fmt.Println("Local signature verification: OK")
			}
		} else {
			fmt.Println("Could not derive public key for local verification")
		}
	}

	// 5. HTTP request
	var bodyReader io.Reader
	if bodyStr != "" {
		bodyReader = strings.NewReader(bodyStr)
	}

	req, err := http.NewRequest(httpMethod, apiPath, bodyReader)
	if err != nil {
		return result, err
	}

	if bodyStr != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Add("X-Revx-Timestamp", timestamp)
	req.Header.Add("X-Revx-Signature", signature)
	if printInfo {
		fmt.Printf("Timestamp: %s\n", timestamp)
		fmt.Printf("Signature: %s\n", signature)
	}

	apiKey, err := os.ReadFile("api_rev_x.pem")
	if err != nil {
		return result, err
	}
	req.Header.Set("X-Revx-API-Key", string(bytes.TrimSpace(apiKey)))
	if printInfo {
		fmt.Printf("API Key: %s\n", string(bytes.TrimSpace(apiKey)))
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		bodyBytes, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return result, fmt.Errorf("http error: %d (failed to read body: %w)", resp.StatusCode, readErr)
		}

		var apiErr struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(bodyBytes, &apiErr); err != nil {
			// fallback: return raw body if it's not valid JSON
			return result, fmt.Errorf("http error: %d: %s", resp.StatusCode, string(bodyBytes))
		}

		return result, fmt.Errorf("http error: %d: %s", resp.StatusCode, apiErr.Message)
	}

	// 6. Decode JSON into T
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return result, err
	}

	return result, nil
}
