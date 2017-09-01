package main

// TODO add check for grounded users to TestMessage once implemented
// TODO only allow one response per 10 seconds
// TODO add some check to block attempts at blank responses ("dad, ground " breaks it, "im ." breaks it too)

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
	Timeout		int // Timeout between multi-lined reply
}

type SpeakData struct {
	Regex 		string
	Response	[]ResponseData
}

type ResponseData struct {
	Message 	string
	Count 		int
}

type Reply struct {
	Content		[]string
	Sent 		time.Time
}

var conf = initConfig()
var serv = flag.String("server", conf.Ip + ":6667", "hostname and port for irc server to connect to")
var nick = flag.String("nick", conf.DadName, "nickname for the bot")
var lastReply Reply

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
	if (r.MatchString(message.Content) && !stringInSlice(message.From, conf.Grounded)) {
		match = true
	}
	return match
}

func stringInSlice (a string, s []string) bool {
	for _, b := range s {
		if (a == b) {
			return true
		}
	}
	return false
}

func formatReply (m *hbot.Message, s SpeakData) Reply {
	var reply Reply
	response := s.Response[rand.Intn(len(s.Response))]
	if (strings.Contains(response.Message, "[from]")) {
		response.Message = strings.Replace(response.Message, "[from]", m.From, -1)
	}
	if (strings.Contains(response.Message, "[grounded]")) {
		response.Message = strings.Replace(response.Message, "[grounded]", strings.Join(conf.Grounded, ", "), -1)
	}

	// Manages all responses that reuse any content from the original message
	for _, replace := range ([]string {"[mock]", "[repeat]", "[user]"}) {
		if (strings.Contains(response.Message, replace)) {
			r := regexp.MustCompile(s.Regex)
			temp := r.Split(m.Content, -1)
			newStr := temp[len(temp) - 1]
			nonWord := regexp.MustCompile("^\\W+$")
			if (len(newStr) == 0 || nonWord.MatchString(newStr)) {
				response.Message = "" // Delete response if newStr is empty
			} else {
				response.Message = strings.Replace(response.Message, replace, newStr, -1)
			}
		}
	}
	reply.Content = strings.Split(response.Message, "\n")
	return reply
}

var GlobalTrigger = hbot.Trigger {
	func (bot *hbot.Bot, m *hbot.Message) bool {
		return true
	},
	func (irc *hbot.Bot, m *hbot.Message) bool {
		for _, r := range conf.Speak {
			if (testMessage(r.Regex, m)) {
				reply := formatReply(m, r)
				reply.Sent = time.Now()
				numSent := 0
				for _, line := range reply.Content {
					if (len(line) > 0) {
						irc.Reply(m, fmt.Sprintf(line))
						numSent++
					}
					if (len(reply.Content) > 1 && numSent > 0) {
						time.Sleep(time.Duration(conf.Timeout) * time.Second)
					}	
				}
				break
			}
		}
		return false
	},
}
