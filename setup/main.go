package main

import (
	"bufio"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"strings"
)

const (
	privateKeyFile = "private.pem"
	publicKeyFile  = "public.pem"
	apiKeyFile     = "api_rev_x.pem"
	exchangeURL    = "https://exchange.revolut.com/home"
)

func main() {
	reader := bufio.NewReader(os.Stdin)

	regenerate := true
	if fileExists(privateKeyFile) || fileExists(publicKeyFile) {
		fmt.Printf("%s et/ou %s existent déjà.\n", privateKeyFile, publicKeyFile)
		fmt.Print("Régénérer une nouvelle paire de clés (invalide l'ancienne chez Revolut) ? (o/N) : ")
		answer, _ := reader.ReadString('\n')
		regenerate = strings.ToLower(strings.TrimSpace(answer)) == "o"
	}

	if regenerate {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			panic(err)
		}
		if err := writePEM(privateKeyFile, "PRIVATE KEY", mustMarshalPKCS8(priv), 0600); err != nil {
			panic(err)
		}
		if err := writePEM(publicKeyFile, "PUBLIC KEY", mustMarshalPKIX(pub), 0644); err != nil {
			panic(err)
		}
		fmt.Printf("Clés générées : %s, %s\n\n", privateKeyFile, publicKeyFile)
	}

	publicKeyPem, err := os.ReadFile(publicKeyFile)
	if err != nil {
		panic(err)
	}

	fmt.Println("Contenu de la clé publique à ajouter sur Revolut :")
	fmt.Println(string(publicKeyPem))

	fmt.Println("Étapes suivantes :")
	fmt.Printf("1. Rendez vous sur votre profile Revolut X : %s\n", exchangeURL)
	fmt.Println("2. Cliquez sur votre photo de profil puis Clés API.")
	fmt.Println("3. Ajoutez la clé publique ci-dessus.")
	fmt.Println("4. Revolut vous fournira une clé d'API : collez-la ci-dessous.")
	fmt.Print("\nClé d'API : ")

	apiKey, err := reader.ReadString('\n')
	if err != nil {
		panic(err)
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		fmt.Printf("Aucune clé fournie, %s non modifié.\n", apiKeyFile)
		return
	}

	if err := os.WriteFile(apiKeyFile, []byte(apiKey+"\n"), 0600); err != nil {
		panic(err)
	}
	fmt.Printf("Clé d'API enregistrée dans %s\n", apiKeyFile)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func writePEM(path, blockType string, der []byte, perm os.FileMode) error {
	block := &pem.Block{Type: blockType, Bytes: der}
	return os.WriteFile(path, pem.EncodeToMemory(block), perm)
}

func mustMarshalPKCS8(priv ed25519.PrivateKey) []byte {
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		panic(err)
	}
	return der
}

func mustMarshalPKIX(pub ed25519.PublicKey) []byte {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		panic(err)
	}
	return der
}
