package main

// TODO change the grounded regex to respond to any form of dad
// TODO initialize all regexp values as global variables and pass those instead of regex strings
// TODO add check for grounded users to TestMessage once implemented
// TODO there needs to be a way to determine which one to follow through on if multiple triggers are activated
	// TODO only allow one response per 10 seconds
	// TODO maybe add a send queue that collects replies (each having a specific priority)
	// and then only sending the one with highest priority and then clearing out the rest
	// (race condition will emerge)
// TODO asking a question/saying good morning shouldn't be triggered unless dad is mentioned in the question

import (
	"flag"
	"fmt"
	// "errors"
	"regexp"
	"encoding/json"
	"os"

	"github.com/whyrusleeping/hellabot"
	log "gopkg.in/inconshreveable/log15.v2"
)

type Configuration struct {
	Admin 		string
	Channels 	[]string
	DadName		string
	Debug		string
	Grounded	[]string
	Ip 			string
	MessageRate int // Using 1 reply per x seconds instead of y per x seconds
	MomName		string
	Timeout		int
}

type Joke struct {
	Setup string
	Punch string
	Count int
}

// Initialize bot config
file, _ := os.Open("conf.json")
decoder := json.NewDecoder(file)
conf := Configuration{}
err := decoder.Decode(&conf)
if err != nil {
	panic(err)
}

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
	irc.AddTrigger(GroundedListTrigger)
	irc.AddTrigger(GoodMorningTrigger)
	irc.AddTrigger(QuestionTrigger)
	irc.Logger.SetHandler(log.StdoutHandler)

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
		r := regexp.MustCompile(`(?i)(?:^|\W+)(i'?m)\W+`)
		reply := r.Split(m.Content, 2)
		r = regexp.MustCompile(`(?i)\s*(a|an)\s+`)
		reply = r.Split(reply[len(reply) - 1], 2)
		irc.Reply(m, fmt.Sprintf("Hi %s, I'm dad.", reply[len(reply) - 1]))
		return false
	},
}

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

var QuestionTrigger = hbot.Trigger {
	func (bot *hbot.Bot, m *hbot.Message) bool {
		return testMessage([]string {`(?i)\?$`}, m)
	},
	func (irc *hbot.Bot, m *hbot.Message) bool {
		irc.Reply(m, "Ask your mother.")
		return false
	},
}

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
