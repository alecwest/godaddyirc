package main

// TODO change the grounded regex to respond to any form of dad
// TODO initialize all regexp values as global variables and pass those instead of regex strings
// TODO add check for grounded users to TestMessage once implemented

import (
	"flag"
	"fmt"
	// "errors"
	"regexp"

	"github.com/whyrusleeping/hellabot"
	log "gopkg.in/inconshreveable/log15.v2"
)

var serv = flag.String("server", "localhost:6667", "hostname and port for irc server to connect to")
var nick = flag.String("nick", "dad", "nickname for the bot")

func main() {
	flag.Parse()

	hijackSession := func(bot *hbot.Bot) {
		bot.HijackSession = true
	}
	channels := func(bot *hbot.Bot) {
		bot.Channels = []string{"#main"}
	}
	irc, err := hbot.NewBot(*serv, *nick, hijackSession, channels)
	if err != nil {
		panic(err)
	}

	irc.AddTrigger(HiImDadTrigger)
	irc.AddTrigger(GoodMorningTrigger)
	irc.Logger.SetHandler(log.StdoutHandler)
	// logHandler := log.LvlFilterHandler(log.LvlInfo, log.StdoutHandler)
	// or
	// irc.Logger.SetHandler(logHandler)
	// or
	// irc.Logger.SetHandler(log.StreamHandler(os.Stdout, log.JsonFormat()))

	// Start up bot (this blocks until we disconnect)
	irc.Run()
	fmt.Println("Bot shutting down.")
}

func testMessage(regexList []string, message *hbot.Message) bool {
	match := false
	// err = errors.New("Forgot to include who the message was from")
	for _, regex := range regexList {
		r := regexp.MustCompile(regex)
		if (r.MatchString(message.Content)) {
			match = true
			break
		}
	}
	return match
}

var HiImDadTrigger = hbot.Trigger {
	func (bot *hbot.Bot, m *hbot.Message) bool {
		// test := []string{"yeet", "a"}
		// return m.Command == "PRIVMSG" && m.Content == "yeet"
		return testMessage([]string {`(?i)(^|\W+)i(')?m(\s\w.*$)`}, m)
	},
	func (irc *hbot.Bot, m *hbot.Message) bool {
		r := regexp.MustCompile(`(?i)(?:^|\W+)(i'?m)\W+`).Split(m.Content, 2)
		irc.Reply(m, fmt.Sprintf("Hi %s, I'm dad.", r[1]))
		return false
	},
}

// TODO there needs to be a way to determine which one to follow through on if multiple triggers are activated

var GroundedListTrigger = hbot.Trigger {
	func (bot *hbot.Bot, m *hbot.Message) bool {
		return testMessage([]string {`(?i)dad, grounded`}, m)
	},
	func (irc *hbot.Bot, m *hbot.Message) bool {
		irc.Reply(m, "Here is a list of grounded users:")
		irc.Reply(m, "//TODO fill this in")
		return false
	},
}

var GoodMorningTrigger = hbot.Trigger {
	func (bot *hbot.Bot, m *hbot.Message) bool {
		return testMessage([]string {`(?i)^(good)?\s?mornin(g)?(,)?`}, m)
	},
	func (irc *hbot.Bot, m *hbot.Message) bool {
		irc.Reply(m, fmt.Sprintf("good morning %s!", m.From))
		return false
	},
}

// var QuestionTrigger = hbot.Trigger {
// }

// var JokeTrigger = hbot.Trigger {
// }

/*
	Copy as needed

	func (bot *hbot.Bot, m *hbot.Message) bool {
	
	},
	func (irc *hbot.Bot, m *hbot.Message) bool {
		return false
	},

*/
