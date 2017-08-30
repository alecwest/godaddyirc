package main

// TODO add check for grounded users to TestMessage once implemented
// TODO only allow one response per 10 seconds
// TODO finish format of replies. Probably need to edit some existing regex so it can also be used to split the string apart from the part that needs formatting

import (
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/whyrusleeping/hellabot"
	log "gopkg.in/inconshreveable/log15.v2"
)

type Configuration struct {
	Admin 		string
	Channels 	[]string
	DadName		string
	Debug		bool
	Grounded	[]string
	Ip 			string
	MessageRate int // Using 1 reply per x seconds instead of y per x seconds
	MomName		string
	Speak 		[]SpeakData
	Timeout		int
}

type SpeakData struct {
	Regex 		string
	Response	[]ResponseData
}

type ResponseData struct {
	Message 	string
	Count 		int
}

var conf = initConfig()
var serv = flag.String("server", conf.Ip + ":6667", "hostname and port for irc server to connect to")
var nick = flag.String("nick", conf.DadName, "nickname for the bot")

func main() {
	rand.Seed(time.Now().Unix())
	flag.Parse()

	hijackSession := func(bot *hbot.Bot) {
		bot.HijackSession = true
	}
	channels := func(bot *hbot.Bot) {
		bot.Channels = conf.Channels
	}
	irc, err := hbot.NewBot(*serv, *nick, hijackSession, channels)
	if err != nil {
		panic(err)
	}

	irc.AddTrigger(GlobalTrigger)
	// irc.AddTrigger(HiImDadTrigger)
	// irc.AddTrigger(GroundedListTrigger)
	// irc.AddTrigger(GoodMorningTrigger)
	// irc.AddTrigger(QuestionTrigger)
	irc.Logger.SetHandler(log.StdoutHandler)

	// Start up bot (this blocks until we disconnect)
	irc.Run()
	fmt.Println("Bot shutting down.")
}

func initConfig () Configuration {
	// Initialize bot config
	file, _ := os.Open("conf.json")
	decoder := json.NewDecoder(file)
	conf := Configuration{}
	err := decoder.Decode(&conf)
	if err != nil {
		panic(err)
	}
	return conf
}

func testMessage (regex string, message *hbot.Message) bool {
	match := false
	// err = errors.New("Forgot to include who the message was from")
	r := regexp.MustCompile(regex)
	if (r.MatchString(message.Content)) {
		match = true
	}
	return match
}


func formatReply (m *hbot.Message, r []ResponseData) []string {
	reply := r[rand.Intn(len(r))]
	reply.Message = strings.Replace(reply.Message, "[from]", m.From, -1)
	// reply.Message = strings.Replace(reply.Message, "[user]", fill_this_in)
	reply.Message = strings.Replace(reply.Message, "[repeat]", m.Content, -1)
	reply.Message = strings.Replace(reply.Message, "[grounded]", strings.Join(conf.Grounded, ", "), -1)
	formattedReply := strings.Split(reply.Message, "\n")
	return formattedReply
}

var GlobalTrigger = hbot.Trigger {
	func (bot *hbot.Bot, m *hbot.Message) bool {
		return true
	},
	func (irc *hbot.Bot, m *hbot.Message) bool {
		for _, r := range conf.Speak {
			if (testMessage(r.Regex, m)) {
				reply := formatReply(m, r.Response)
				for _, line := range reply {
					irc.Reply(m, fmt.Sprintf(line))
					if (len(reply) > 1) {
						time.Sleep(time.Duration(conf.Timeout) * time.Second)
					}	
				}
				break
			}
		}
		return false
	},
}
