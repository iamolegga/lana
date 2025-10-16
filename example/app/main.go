package main

import (
	"context"
	"crypto/rsa"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/securecookie"
	"github.com/lestrrat-go/jwx/v2/jwk"
)

var (
	parentHostname string
	cookieSecret   []byte
	appPort        string
	sc             *securecookie.SecureCookie
)

const (
	cookieName = "session"
)

func main() {
	parentHostname = getEnv("PARENT_HOSTNAME")
	cookieSecret = []byte(getEnv("COOKIE_SECRET"))
	appPort = getEnv("APP_PORT")

	if len(cookieSecret) < 32 {
		log.Fatal("COOKIE_SECRET must be at least 32 characters for security")
	}

	sc = securecookie.New(cookieSecret, nil)

	http.HandleFunc("/", handleRoot)
	http.HandleFunc("/callback", handleCallback)
	http.HandleFunc("/logout", handleLogout)

	log.Printf("Example app starting on port %s", appPort)
	log.Printf("Parent hostname: %s", parentHostname)

	if err := http.ListenAndServe(":"+appPort, nil); err != nil {
		log.Fatal(err)
	}
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(cookieName)
	if err != nil || cookie.Value == "" {
		redirectToLANA(w, r)
		return
	}

	var email string
	if err := sc.Decode(cookieName, cookie.Value, &email); err != nil {
		redirectToLANA(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <title>LANA Example - Authenticated</title>
    <style>
        body {
            font-family: Arial, sans-serif;
            max-width: 600px;
            margin: 50px auto;
            padding: 20px;
            background-color: #f5f5f5;
        }
        .container {
            background: white;
            padding: 30px;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        h1 { color: #4CAF50; }
        .email {
            background: #e8f5e9;
            padding: 10px;
            border-radius: 4px;
            margin: 20px 0;
            word-break: break-all;
        }
        .logout {
            display: inline-block;
            background: #f44336;
            color: white;
            padding: 10px 20px;
            text-decoration: none;
            border-radius: 4px;
            margin-top: 20px;
        }
        .logout:hover { background: #da190b; }
    </style>
</head>
<body>
    <div class="container">
        <h1>✓ Authentication Successful!</h1>
        <p>You are authenticated as:</p>
        <div class="email"><strong>%s</strong></div>
        <p>This example app received a JWT from LANA, verified its signature, and stored your email in a signed cookie.</p>
        <a href="/logout" class="logout">Logout</a>
    </div>
</body>
</html>`, email)
}

func handleCallback(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "Missing token parameter", http.StatusBadRequest)
		return
	}

	email, err := JWTVerify(token, parentHostname)
	if err != nil {
		log.Printf("Error verifying JWT: %v", err)
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return
	}

	encoded, err := sc.Encode(cookieName, email)
	if err != nil {
		log.Printf("Error encoding cookie: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    encoded,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   3600, // 1 hour
		Domain:   r.Host,
	})

	http.Redirect(w, r, "/", http.StatusFound)
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Domain:   r.Host,
	})
	http.Redirect(w, r, "/", http.StatusFound)
}

func redirectToLANA(w http.ResponseWriter, r *http.Request) {
	loginURL := fmt.Sprintf(
		"https://auth.%s?redirect=%s",
		parentHostname,
		url.QueryEscape(
			fmt.Sprintf("https://%s/callback", r.Host),
		),
	)
	http.Redirect(w, r, loginURL, http.StatusFound)
}

func JWTVerify(tokenStr, parentHostname string) (string, error) {
	if tokenStr == "" {
		return "", errors.New("empty token")
	}

	parser := jwt.NewParser()
	claims := jwt.MapClaims{}
	_, _, err := parser.ParseUnverified(tokenStr, claims)
	if err != nil {
		return "", fmt.Errorf("invalid token format: %w", err)
	}

	issuer, err := claims.GetIssuer()
	if err != nil || issuer == "" {
		return "", errors.New("invalid issuer claim")
	}
	issuerURL, err := url.Parse(issuer)
	if err != nil {
		return "", fmt.Errorf("invalid issuer URL: %w", err)
	}

	issuerHostname := issuerURL.Hostname()
	if !strings.HasSuffix(issuerHostname, parentHostname) {
		return "", errors.New("not trusted issuer")
	}
	publicKey, err := fetchPublicKey(issuer)
	if err != nil {
		return "", fmt.Errorf("failed to fetch public key: %w", err)
	}

	token, err := jwt.Parse(
		tokenStr,
		func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
				return nil, fmt.Errorf(
					"unexpected signing method: %v",
					token.Header["alg"],
				)
			}

			return publicKey, nil
		},
		jwt.WithValidMethods([]string{"RS256"}),
	)

	if err != nil {
		return "", fmt.Errorf("invalid token: %w", err)
	}

	if !token.Valid {
		return "", errors.New("invalid token")
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", errors.New("invalid token claims")
	}

	exp, err := claims.GetExpirationTime()
	if err != nil || exp == nil || exp.Before(time.Now()) {
		return "", errors.New("token expired")
	}
	sub, err := claims.GetSubject()
	if err != nil || sub == "" {
		return "", errors.New("invalid subject claim")
	}

	return sub, nil
}

func fetchPublicKey(issuer string) (*rsa.PublicKey, error) {
	wellKnownURL := issuer
	if !strings.HasSuffix(wellKnownURL, "/") {
		wellKnownURL += "/"
	}
	wellKnownURL += ".well-known/jwks.json"

	set, err := jwk.Fetch(context.Background(), wellKnownURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch JWKS: %w", err)
	}

	// Look for a suitable RSA key
	var rsaKey rsa.PublicKey
	for i := 0; i < set.Len(); i++ {
		key, _ := set.Key(i)

		// Check if it's an RSA key with the right usage
		if key.KeyType() == "RSA" &&
			(key.Algorithm().String() == "RS256" || key.KeyUsage() == "sig") {
			// Extract the raw RSA key
			if err := key.Raw(&rsaKey); err == nil {
				return &rsaKey, nil
			}
		}
	}

	return nil, fmt.Errorf("no suitable RSA key found in JWKS")
}

func getEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatalf("Environment variable %s is required but not set", key)
	}
	return value
}
