// © 2013 the CatBase Authors under the WTFPL. See AUTHORS for the list of authors.

package bot

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Handles incomming PRIVMSG requests
func (b *Bot) MsgReceived(msg Message) {
	log.Println("Received message: ", msg)

	// msg := b.buildMessage(client, inMsg)
	// do need to look up user and fix it

	if strings.HasPrefix(msg.Body, "help") && msg.Command {
		parts := strings.Fields(strings.ToLower(msg.Body))
		b.checkHelp(msg.Channel, parts)
		goto RET
	}

	for _, name := range b.PluginOrdering {
		p := b.Plugins[name]
		if p.Message(msg) {
			break
		}
	}

RET:
	b.logIn <- msg
	return
}

// Handle incoming events
func (b *Bot) EventReceived(msg Message) {
	log.Println("Received event: ", msg)
	//msg := b.buildMessage(conn, inMsg)
	for _, name := range b.PluginOrdering {
		p := b.Plugins[name]
		if p.Event(msg.Body, msg) { // TODO: could get rid of msg.Body
			break
		}
	}
}

func (b *Bot) SendMessage(channel, message string) {
	b.Conn.SendMessage(channel, message)
}

func (b *Bot) SendAction(channel, message string) {
	b.Conn.SendAction(channel, message)
}

// Checks to see if the user is asking for help, returns true if so and handles the situation.
func (b *Bot) checkHelp(channel string, parts []string) {
	if len(parts) == 1 {
		// just print out a list of help topics
		topics := "Help topics: about variables"
		for name, _ := range b.Plugins {
			topics = fmt.Sprintf("%s, %s", topics, name)
		}
		b.SendMessage(channel, topics)
	} else {
		// trigger the proper plugin's help response
		if parts[1] == "about" {
			b.Help(channel, parts)
			return
		}
		if parts[1] == "variables" {
			b.listVars(channel, parts)
			return
		}
		plugin := b.Plugins[parts[1]]
		if plugin != nil {
			plugin.Help(channel, parts)
		} else {
			msg := fmt.Sprintf("I'm sorry, I don't know what %s is!", parts[1])
			b.SendMessage(channel, msg)
		}
	}
}

func (b *Bot) LastMessage(channel string) (Message, error) {
	log := <-b.logOut
	if len(log) == 0 {
		return Message{}, errors.New("No messages found.")
	}
	for i := len(log) - 1; i >= 0; i-- {
		msg := log[i]
		if strings.ToLower(msg.Channel) == strings.ToLower(channel) {
			return msg, nil
		}
	}
	return Message{}, errors.New("No messages found.")
}

// Take an input string and mutate it based on $vars in the string
func (b *Bot) Filter(message Message, input string) string {
	rand.Seed(time.Now().Unix())

	if strings.Contains(input, "$NICK") {
		nick := strings.ToUpper(message.User.Name)
		input = strings.Replace(input, "$NICK", nick, -1)
	}

	// Let's be bucket compatible for this var
	input = strings.Replace(input, "$who", "$nick", -1)
	if strings.Contains(input, "$nick") {
		nick := message.User.Name
		input = strings.Replace(input, "$nick", nick, -1)
	}

	for strings.Contains(input, "$someone") {
		nicks := b.Who(message.Channel)
		someone := nicks[rand.Intn(len(nicks))].Name
		input = strings.Replace(input, "$someone", someone, 1)
	}

	for strings.Contains(input, "$digit") {
		num := strconv.Itoa(rand.Intn(9))
		input = strings.Replace(input, "$digit", num, 1)
	}

	for strings.Contains(input, "$nonzero") {
		num := strconv.Itoa(rand.Intn(8) + 1)
		input = strings.Replace(input, "$nonzero", num, 1)
	}

	r, err := regexp.Compile("\\$[A-z]+")
	if err != nil {
		panic(err)
	}

	varname := r.FindString(input)
	blacklist := make(map[string]bool)
	blacklist["$and"] = true
	for len(varname) > 0 && !blacklist[varname] {
		text, err := b.getVar(varname)
		if err != nil {
			blacklist[varname] = true
			continue
		}
		input = strings.Replace(input, varname, text, 1)
		varname = r.FindString(input)
	}

	return input
}

func (b *Bot) getVar(varName string) (string, error) {
	var text string
	err := b.DB.QueryRow(`select v.value from variables as va inner join "values" as v on va.id = va.id = v.varId order by random() limit 1`).Scan(&text)
	switch {
	case err == sql.ErrNoRows:
		return "", fmt.Errorf("No factoid found")
	case err != nil:
		log.Fatal("getVar error: ", err)
	}
	return text, nil
}

func (b *Bot) listVars(channel string, parts []string) {
	rows, err := b.DB.Query(`select name from variables`)
	if err != nil {
		log.Fatal(err)
	}
	msg := "I know: $who, $someone, $digit, $nonzero"
	for rows.Next() {
		var variable string
		err := rows.Scan(&variable)
		if err != nil {
			log.Println("Error scanning variable.")
			continue
		}
		msg = fmt.Sprintf("%s, %s", msg, variable)
	}
	b.SendMessage(channel, msg)
}

func (b *Bot) Help(channel string, parts []string) {
	msg := fmt.Sprintf("Hi, I'm based on godeepintir version %s. I'm written in Go, and you "+
		"can find my source code on the internet here: "+
		"http://github.com/velour/catbase", b.Version)
	b.SendMessage(channel, msg)
}

// Send our own musings to the plugins
func (b *Bot) selfSaid(channel, message string, action bool) {
	msg := Message{
		User:    &b.Me, // hack
		Channel: channel,
		Body:    message,
		Raw:     message, // hack
		Action:  action,
		Command: false,
		Time:    time.Now(),
		Host:    "0.0.0.0", // hack
	}

	for _, name := range b.PluginOrdering {
		p := b.Plugins[name]
		if p.BotMessage(msg) {
			break
		}
	}
}
