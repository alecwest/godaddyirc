package main

// TODO add check for grounded users to TestMessage once implemented
// TODO only allow one response per 10 seconds
// TODO add some check to block attempts at blank responses ("dad, ground " breaks it, "im ." breaks it too)
// TODO in formatReply, if the [] block gets replaced with an empty or non-character string, make the entire reply empty so a check later on doesn't allow it to send.

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

func formatReply (m *hbot.Message, s SpeakData) []string {
	reply := s.Response[rand.Intn(len(s.Response))]
	if (strings.Contains(reply.Message, "[from]")) {
		reply.Message = strings.Replace(reply.Message, "[from]", m.From, -1)
	}
	if (strings.Contains(reply.Message, "[grounded]")) {
		reply.Message = strings.Replace(reply.Message, "[grounded]", strings.Join(conf.Grounded, ", "), -1)
	}
	if (strings.Contains(reply.Message, "[mock]")) {
		r := regexp.MustCompile(s.Regex)
		temp := r.Split(m.Content, -1)
		mock := temp[len(temp) - 1]
		reply.Message = strings.Replace(reply.Message, "[mock]", mock, -1)
	}
	if (strings.Contains(reply.Message, "[repeat]")) {
		r := regexp.MustCompile(s.Regex)
		temp := r.Split(m.Content, -1)
		repeat := temp[len(temp) - 1]
		reply.Message = strings.Replace(reply.Message, "[repeat]", repeat, -1)
	}
	if (strings.Contains(reply.Message, "[user]")) {
		r := regexp.MustCompile(s.Regex)
		temp := r.Split(m.Content, -1)
		user := temp[len(temp) - 1]
		reply.Message = strings.Replace(reply.Message, "[user]", user, -1)
	}
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
				// TODO add line here to record time of last reply for MessageRate
				reply := formatReply(m, r)
				sent := 0
				for _, line := range reply {
					if (len(line) > 0) {
						irc.Reply(m, fmt.Sprintf(line))
						sent++
					}
					if (len(reply) > 1 && sent > 0) {
						time.Sleep(time.Duration(conf.Timeout) * time.Second)
					}	
				}
				break
			}
		}
		return false
	},
}
