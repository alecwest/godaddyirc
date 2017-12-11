package main

// TODO move last reply to config
// TODO dad, say is broken
// TODO refactor stuff that manages the config (make a config navigation struct or something)
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

type DadBot struct {
	Dad 		*hbot.Bot
	Conf 		Configuration
	LastReply 	Reply
}

var dbot DadBot

func main() {
	rand.Seed(time.Now().Unix())
	flag.Parse()
	dbot.Conf = initConfig()
	serv := flag.String("server", dbot.Conf.Ip + ":6667", "hostname and port for irc server to connect to")
	nick := flag.String("nick", dbot.Conf.DadName, "nickname for the bot")

	hijackSession := func(bot *hbot.Bot) {
		bot.HijackSession = true
	}
	channels := func(bot *hbot.Bot) {
		bot.Channels = dbot.Conf.Channels
	}
	bot, err := hbot.NewBot(*serv, *nick, hijackSession, channels)
	dbot.Dad = bot
	if err != nil {
		panic(err)
	}

	dbot.Dad.AddTrigger(UserTrigger)
	dbot.Dad.AddTrigger(AdminTrigger)
	dbot.Dad.Logger.SetHandler(log.StdoutHandler)

	// Start up bot (this blocks until we disconnect)
	dbot.Dad.Run()
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
	jsonData, err := json.MarshalIndent(dbot.Conf, "", "    ")
	if (err != nil) {
		panic(err)
	}
	ioutil.WriteFile("conf.json", jsonData, 0644)
}

func updateGrounding (content string, command string) {
	i := stringInSlice(content, dbot.Conf.Grounded)

	// log.Debug(fmt.Sprintf("index: %d, grounding/ungrounding: %s", i, content))
	if (command == "[ground]" && i == -1) {
		dbot.Conf.Grounded = append(dbot.Conf.Grounded, content)
	} else if (command == "[unground]" && i != -1) {
		dbot.Conf.Grounded[len(dbot.Conf.Grounded) - 1], dbot.Conf.Grounded[i] = dbot.Conf.Grounded[i], dbot.Conf.Grounded[len(dbot.Conf.Grounded) - 1]
		dbot.Conf.Grounded = dbot.Conf.Grounded[:len(dbot.Conf.Grounded) - 1]
	}
}

func testMessage (regex string, message *hbot.Message) bool {
	match := false
	// err = errors.New("Forgot to include who the message was from")
	r := regexp.MustCompile(regex)
	if (r.MatchString(message.Content) && 
		stringInSlice(message.From, dbot.Conf.Grounded) == -1 &&
		messageRate(message)) {
		match = true
	}
	return match
}

func messageRate (m *hbot.Message) bool {
	return (time.Since(dbot.LastReply.Sent) > (time.Duration(dbot.Conf.MessageRate) * time.Second) || m.From == dbot.Conf.Admin)
}

func stringInSlice (a string, s []string) int {
	for i, b := range s {
		if (a == b) {
			return i
		}
	}
	return -1
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

func formatReply (m *hbot.Message, admin_speak bool, s_index int) Reply {
	var s SpeakData
	var reply Reply
	if (admin_speak) {
		s = dbot.Conf.AdminSpeak[s_index]
	} else {
		s = dbot.Conf.Speak[s_index]
	}

	// Choose random response from list of responses (mostly used for jokes)
	var rand_index = rand.Intn(len(s.Response))
	response := s.Response[rand_index]
	
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
		response.Message = strings.Replace(response.Message, "[grounded]", strings.Join(dbot.Conf.Grounded, ", "), -1)
	}

	// Manages all responses that reuse any content from the original message
	for _, replace := range ([]string {"[mock]", "[repeat]", "[ground]", "[unground]"}) {
		if (strings.Contains(response.Message, replace)) {
			
			// Modify who the message is sent to if it includes "user:" before the command
			if (replace == "[repeat]") {
				to := setRecipient(m, s)
				if (len(to) > 0) {
					reply.To = to
				}
			} else {
				// Remove the part that the regex matched to
				m.Content = removeRegex(m.Content, s.Regex)
			}

			// Manage grounding/ungrounding
			updateGrounding(m.Content, replace)


			// Replace [...] element in the response with what remains in the Content of the message
			nonWord := regexp.MustCompile("^\\W+$")
			if (len(m.Content) == 0 || nonWord.MatchString(m.Content)) {
				response.Message = "" // Delete response if m.Content is empty
			} else {
				response.Message = strings.Replace(response.Message, replace, m.Content, -1)
			}
		}
	}
	if (response.Message != "") {
		if (admin_speak) {
			dbot.Conf.AdminSpeak[s_index].Response[rand_index].Count++
		} else {
			dbot.Conf.Speak[s_index].Response[rand_index].Count++
		}
	}
	reply.Content = strings.Split(response.Message, "\n")
	return reply
}

func performAction (irc *hbot.Bot, m *hbot.Message, admin_speak bool) bool {
	var speak []SpeakData
	if (admin_speak) {
		speak = dbot.Conf.AdminSpeak
	} else {
		speak = dbot.Conf.Speak
	}
	for i, s := range speak {
		if (testMessage(s.Regex, m)) {
			reply := formatReply(m, admin_speak, i)
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
					time.Sleep(time.Duration(dbot.Conf.Timeout) * time.Second)
				}
			}
			if (numSent > 0) {
				// Record last sent message
				dbot.LastReply = reply
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
		return (m.From != dbot.Conf.Admin)
	},
	func (irc *hbot.Bot, m *hbot.Message) bool {
		_ = performAction(irc, m, false)
		updateConfig()
		return false
	},
}

var AdminTrigger = hbot.Trigger {
	func (bot *hbot.Bot, m *hbot.Message) bool {
		return (m.From == dbot.Conf.Admin)
	},
	func (irc *hbot.Bot, m *hbot.Message) bool {
		responded := performAction(irc, m, true)
		if (!responded) {
			_ = performAction(irc, m, false)
		}
		updateConfig()
		return false
	},
}
