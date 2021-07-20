package plugin

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"log"
	"net/url"
	"time"

	"github.com/pkg/errors"
)

const expiryTimeLayout = "2006-01-02T15:04:05.000Z"

// SignURL takes in a URL and adds a sha1 signature and expiration to it.
// Namespace is used to create or get the signing key from a k8s secret.
func SignURL(signedUrl *url.URL, namespace string, ttl time.Duration) error {

	expiration := time.Now().Add(ttl)
	signedUrl.RawQuery += fmt.Sprintf("expires=%s", url.QueryEscape(expiration.Format(expiryTimeLayout)))

	signingKey, err := getSigningKey(namespace)
	if err != nil {
		return errors.Wrap(err, "failed to get signing key")
	}

	mac := hmac.New(sha1.New, signingKey)
	mac.Write([]byte(signedUrl.String()))
	sig := base64.URLEncoding.EncodeToString(mac.Sum(nil))
	signedUrl.RawQuery += fmt.Sprintf("&signature=%s", sig)

	return nil
}

// IsSignedURL validates the expiration and signature of a signed url.
// Namespace is used to get the signing key from a k8s secret.
func IsSignedURLValid(requestURL, namespace string) (bool, error) {
	parsedURL, err := url.Parse(requestURL)
	if err != nil {
		return false, errors.Wrap(err, "failed to parse URL")
	}

	queryParams := parsedURL.Query()

	expiredQueryParam := queryParams.Get("expires")
	if expiredQueryParam == "" {
		return false, nil
	}

	log.Print(expiredQueryParam)

	expirationTime, err := time.Parse(expiryTimeLayout, expiredQueryParam)
	if err != nil {
		return false, errors.Wrap(err, "failed to parse expiration time")
	}

	if expirationTime.Before(time.Now()) {
		return false, err
	}

	encodedHash := queryParams.Get("signature")
	if encodedHash == "" {
		return false, nil
	}

	signingKey, err := getSigningKey(namespace)
	if err != nil {
		return false, errors.Wrap(err, "failed to get signing key")
	}

	messageMACBuf, err := base64.URLEncoding.DecodeString(encodedHash)
	if err != nil {
		return false, errors.Wrap(err, "failed to decode hash")
	}

	// Remove signature from URL and validate
	queryParams.Del("signature")
	parsedURL.RawQuery = queryParams.Encode()
	valid := CheckMAC([]byte(parsedURL.String()), []byte(messageMACBuf), signingKey)
	if !valid {
		return false, nil
	}
	return true, nil
}

// CheckMAC verifies hash checksum
func CheckMAC(message, messageMAC, key []byte) bool {
	mac := hmac.New(sha1.New, key)
	mac.Write(message)
	expectedMAC := mac.Sum(nil)

	return hmac.Equal(messageMAC, expectedMAC)
}
