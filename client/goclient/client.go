/*
 * Copyright (c) 2013 IBM Corp.
 *
 * All rights reserved. This program and the accompanying materials
 * are made available under the terms of the Eclipse Public License v1.0
 * which accompanies this distribution, and is available at
 * http://www.eclipse.org/legal/epl-v10.html
 *
 * Contributors:
 *    Seth Hoenig
 *    Allan Stockdill-Mander
 *    Mike Robertson
 */

package main

import (
	"encoding/json"
	//"flag"
	"fmt"
	"os"
	//"time"

	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/urfave/cli"
)

/*
Options:
 [-help]                      Display help
 [-q 0|1|2]                   Quality of Service
 [-clean]                     CleanSession (true if -clean is present)
 [-id <clientid>]             CliendID
 [-user <user>]               User
 [-password <password>]       Password
 [-broker <uri>]              Broker URI
 [-topic <topic>]             Topic
 [-store <path>]              Store Directory

*/

//default params
var broker string   // := "tcp://127.0.0.1:1883"
var topic string    //:= "extiot/device"
var clientid string //:= "client"
var deviceid string
var user string     //:= ""
var password string //:= ""
var qos int         //:= 1
var store string    //:= ":memory:"
var cleansess bool  //:= false

func monitAction(c *cli.Context) error {
	store = ":memory:"
	cleansess = false
	qos = 0 //monit just use qos 0

	//deviceid  is necessary
	if deviceid == "" {
		fmt.Println("deviceid is necessary")
		return nil
	}

	fmt.Println("broker = ", broker)
	fmt.Println("topic = ", topic)
	fmt.Println("clientid = ", clientid)
	fmt.Println("deviceid = ", deviceid)
	fmt.Println("user = ", user)
	fmt.Println("password = ", password)
	fmt.Println("qos = ", qos)
	opts := MQTT.NewClientOptions()
	opts.AddBroker(broker)
	opts.SetClientID(clientid)
	opts.SetUsername(user)
	opts.SetPassword(password)
	opts.SetCleanSession(cleansess)
	if store != ":memory:" {
		opts.SetStore(MQTT.NewFileStore(store))
	}

	//init
	choke := make(chan [2]string)
	opts.SetDefaultPublishHandler(func(client MQTT.Client, msg MQTT.Message) {
		choke <- [2]string{msg.Topic(), string(msg.Payload())}
	})

	client := MQTT.NewClient(opts)
	if tokenc := client.Connect(); tokenc.Wait() && tokenc.Error() != nil {
		fmt.Println("connect failed!")
		return tokenc.Error()
	}
	defer client.Disconnect(250)

	//sub
	if tokens := client.Subscribe(topic+"/data/"+deviceid, byte(qos), nil); tokens.Wait() && tokens.Error() != nil {
		fmt.Println("subscribe error!")
		return tokens.Error()
	}
	for {
		incoming := <-choke
		fmt.Printf("RECEIVED TOPIC: %s : %s\n", incoming[0], incoming[1])
		//TODO check product state schema
	}
	return nil
}

//发送一个json给设备
func configAction(c *cli.Context) error {
	store = ":memory:"
	cleansess = false
	qos = 1 // config should use qos 1

	//deviceid  is necessary
	if deviceid == "" {
		fmt.Println("deviceid is necessary")
		return nil
	}

	fmt.Println("broker = ", broker)
	fmt.Println("topic = ", topic)
	fmt.Println("clientid = ", clientid)
	fmt.Println("deviceid = ", deviceid)
	fmt.Println("user = ", user)
	fmt.Println("password = ", password)
	opts := MQTT.NewClientOptions()
	opts.AddBroker(broker)
	opts.SetClientID(clientid)
	opts.SetUsername(user)
	opts.SetPassword(password)
	opts.SetCleanSession(cleansess)
	if store != ":memory:" {
		opts.SetStore(MQTT.NewFileStore(store))
	}

	//init
	//choke := make(chan [2]string)
	//opts.SetDefaultPublishHandler(func(client MQTT.Client, msg MQTT.Message) {
	//	choke <- [2]string{msg.Topic(), string(msg.Payload())}
	//})

	client := MQTT.NewClient(opts)
	if tokenc := client.Connect(); tokenc.Wait() && tokenc.Error() != nil {
		fmt.Println("connect failed!")
		return tokenc.Error()
	}
	defer client.Disconnect(250)

	if c.Args().First() == "" {
		fmt.Println("should has a json config")
		return nil
	}
	//pub
	//do get data
	var obj interface{}
	err := json.Unmarshal([]byte(c.Args().First()), &obj)
	if err != nil {
		fmt.Println("unmarshal error!", err, c.Args().First())
		return err
	}
	config := obj.(map[string]interface{})
	fmt.Println(config) //TODO check product config schema

	b, err := json.Marshal(config)
	if err != nil {
		fmt.Println("error:", err)
		return err
	}
	fmt.Println("publish device info ", string(b), "topic:", topic+"/config/"+deviceid, "qos:", qos)
	token := client.Publish(topic+"/config/"+deviceid, byte(qos), false, b)
	token.Wait()

	//sub
	//if tokens := client.Subscribe(topic+"/"+deviceid, byte(qos), nil); tokens.Wait() && tokens.Error() != nil {
	//	fmt.Println("subscribe error!")
	//	return tokens.Error()
	//}
	//for {
	//	incoming := <-choke
	//	fmt.Printf("RECEIVED TOPIC: %s : %s\n", incoming[0], incoming[1])
	//	//TODO check product state schema
	//}
	return nil
}

func main() {

	app := cli.NewApp()
	app.Usage = "extiot simple client to monit/config device"
	app.Version = "0.0.1"

	//base params
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:        "broker",
			Value:       "tcp://127.0.0.1:1883",
			Usage:       "url for mqtt broker",
			Destination: &broker,
		},
		cli.StringFlag{
			Name:        "topic",
			Value:       "extiot/device",
			Usage:       "topic",
			Destination: &topic,
		},
		cli.StringFlag{
			Name:        "clientid",
			Value:       "client",
			Usage:       "id for mqtt client",
			Destination: &clientid,
		},
		cli.StringFlag{
			Name:        "deviceid",
			Value:       "",
			Usage:       "id for device",
			Destination: &deviceid,
		},
		cli.StringFlag{
			Name:        "user",
			Value:       "",
			Usage:       "user for mqtt client",
			Destination: &user,
		},
		cli.StringFlag{
			Name:        "password",
			Value:       "",
			Usage:       "passwrod for mqtt client",
			Destination: &password,
		},
	}
	//cmd [global options] command [command options] [arguments...]
	//golbal options:  mqtt server

	//
	app.Commands = []cli.Command{
		/*{
			Name:    "list",
			Aliases: []string{"l"},
			Usage:   "list all device",
			Action:  monitAction,
		},*/
		{
			Name:    "monit",
			Aliases: []string{"m"},
			Usage:   "monit a device",
			Action:  monitAction,
		},
		{
			Name:    "config",
			Aliases: []string{"c"},
			Usage:   "complete a task on the list",
			Action:  configAction,
		},
		/*{
			Name:    "template",
			Aliases: []string{"t"},
			Usage:   "options for task templates",
			Subcommands: []cli.Command{
				{
					Name:  "add",
					Usage: "add a new template",
					Action: func(c *cli.Context) error {
						fmt.Println("new task template: ", c.Args().First())
						return nil
					},
				},
				{
					Name:  "remove",
					Usage: "remove an existing template",
					Action: func(c *cli.Context) error {
						fmt.Println("removed task template: ", c.Args().First())
						return nil
					},
				},
			},
		},*/
	}

	app.Run(os.Args)
}
