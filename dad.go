package main

// TODO implement stuff that modifies conf file (grounding, count increments)
// TODO "I love ____.", "Well then why don't you marry it?"


// TODO add attribute for responses that involve reuse (ReuseContent bool)
// TODO replace [...] blocks with %s and put them in a separate attribute (Format string)

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
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
	AdminSpeak	[]SpeakData
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
	To 			string
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

	irc.AddTrigger(UserTrigger)
	irc.AddTrigger(AdminTrigger)
	irc.Logger.SetHandler(log.StdoutHandler)

	// Start up bot (this blocks until we disconnect)
	irc.Run()
	fmt.Println("Bot shutting down.")
}

func initConfig () Configuration {
	// Initialize bot config
	file, _ := os.Open("conf.json")
	defer file.Close()
	decoder := json.NewDecoder(file)
	conf := Configuration{}
	err := decoder.Decode(&conf)
	if err != nil {
		panic(err)
	}
	return conf
}

func updateConfig () {
	jsonData, err := json.MarshalIndent(conf, "", "    ")
	if (err != nil) {
		panic(err)
	}
	ioutil.WriteFile("conf_test.json", jsonData, 0644)
}

func testMessage (regex string, message *hbot.Message) bool {
	match := false
	// err = errors.New("Forgot to include who the message was from")
	r := regexp.MustCompile(regex)
	if (r.MatchString(message.Content) && 
		!stringInSlice(message.From, conf.Grounded) &&
		messageRate(message)) {
		match = true
	}
	return match
}

func messageRate (m *hbot.Message) bool {
	return (time.Since(lastReply.Sent) > (time.Duration(conf.MessageRate) * time.Second) || m.From == conf.Admin)
}

func stringInSlice (a string, s []string) bool {
	for _, b := range s {
		if (a == b) {
			return true
		}
	}
	return false
}

func removeRegex (s string, regex string) string {
	r := regexp.MustCompile(regex)
	return r.ReplaceAllLiteralString(s, "")
}

// Remove both the command and the person/channel to respond to 
func setRecipient (m *hbot.Message, s SpeakData) string {
	to := ""
	strWithoutCommand := removeRegex(m.Content, s.Regex)
	strWithoutName := removeRegex(m.Content, ".*#?\\w+:\\s+")
	if (strWithoutCommand != strWithoutName) {
		to = removeRegex(m.Content, ":.*")
		m.Content = strWithoutName
	} else {
		m.Content = strWithoutCommand
	}
	return to
}

func formatReply (m *hbot.Message, s SpeakData) Reply {
	var reply Reply
	// Choose random response from list of responses (mostly used for jokes)
	response := s.Response[rand.Intn(len(s.Response))]
	
	// Stolen from Bot.Reply to init reply.To
	if strings.Contains(m.To, "#") {
		reply.To = m.To
	} else {
		reply.To = m.From
	}

	if (strings.Contains(response.Message, "[from]")) {
		response.Message = strings.Replace(response.Message, "[from]", m.From, -1)
	}
	if (strings.Contains(response.Message, "[grounded]")) {
		response.Message = strings.Replace(response.Message, "[grounded]", strings.Join(conf.Grounded, ", "), -1)
	}

	// Manages all responses that reuse any content from the original message
	for _, replace := range ([]string {"[mock]", "[repeat]", "[user]"}) {
		if (strings.Contains(response.Message, replace)) {
			// Modify who the message is sent to if it includes "user:" before the command
			if (replace == "[repeat]") {
				to := setRecipient(m, s)
				if (len(to) > 0) {
					reply.To = to
				}
			} else {
				m.Content = removeRegex(m.Content, s.Regex)
			}

			nonWord := regexp.MustCompile("^\\W+$")
			if (len(m.Content) == 0 || nonWord.MatchString(m.Content)) {
				response.Message = "" // Delete response if m.Content is empty
			} else {
				response.Message = strings.Replace(response.Message, replace, m.Content, -1)
			}
		}
	}
	reply.Content = strings.Split(response.Message, "\n")
	return reply
}

func performAction (irc *hbot.Bot, m *hbot.Message, speak []SpeakData) bool {
	for _, r := range speak {
		if (testMessage(r.Regex, m)) {
			reply := formatReply(m, r)
			reply.Sent = time.Now()
			numSent := 0
			for _, line := range reply.Content {
				// Make sure line is non-empty before sending
				if (len(line) > 0) {
					irc.Msg(reply.To, fmt.Sprintf(line))
					numSent++
				}
				// Make sure there is a timeout between multiple lines in a reply
				if (len(reply.Content) > 1 && numSent > 0) {
					time.Sleep(time.Duration(conf.Timeout) * time.Second)
				}
			}
			if (numSent > 0) {
				// Record last sent message
				lastReply = reply
				updateConfig()
				return true
			}
			// If a regex statement passed but nothing was sent, 
			// the loop should not continue trying to match the reply to others.
			break 
		}
	}
	return false
}

var UserTrigger = hbot.Trigger {
	func (bot *hbot.Bot, m *hbot.Message) bool {
		return (m.From != conf.Admin)
	},
	func (irc *hbot.Bot, m *hbot.Message) bool {
		performAction(irc, m, conf.Speak)
		return false
	},
}

var AdminTrigger = hbot.Trigger {
	func (bot *hbot.Bot, m *hbot.Message) bool {
		return (m.From == conf.Admin)
	},
	func (irc *hbot.Bot, m *hbot.Message) bool {
		responded := performAction(irc, m, conf.AdminSpeak)
		if (!responded) {
			performAction(irc, m, conf.Speak)
		}
		return false
	},
}
