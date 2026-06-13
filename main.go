package main

import (
	"os"

	"github.com/GlazKrovi/Sabot-private/bot"
)

func main() {
	bot.Run(os.Args[1:])
}
