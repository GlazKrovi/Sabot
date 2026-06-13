package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/GlazKrovi/Sabot-private/bot"
)

// requiredKeyFiles are the files contactApi reads to sign and authenticate requests.
var requiredKeyFiles = []string{"private.pem", "api_rev_x.pem"}

func main() {
	var missing []string
	for _, f := range requiredKeyFiles {
		if _, err := os.Stat(f); err != nil {
			missing = append(missing, f)
		}
	}
	if len(missing) > 0 {
		fmt.Printf("Fichier(s) manquant(s) : %s\n", strings.Join(missing, ", "))
		fmt.Println("Lancez d'abord 'go run ./setup' pour générer vos clés et configurer votre clé d'API, puis relancez cette commande.")
		os.Exit(1)
	}

	fmt.Println("Vérification de la clé d'API auprès de Revolut X...")
	if _, err := bot.FetchBalances(false); err != nil {
		fmt.Printf("La clé d'API ne fonctionne pas ou n'est pas autorisée : %v\n", err)
		fmt.Println("Lancez 'go run ./setup' pour régénérer vos clés et votre clé d'API, puis relancez cette commande.")
		os.Exit(1)
	}
	fmt.Println("Clé d'API valide. Démarrage du bot de trading...")

	bot.Run(os.Args[1:])
}
