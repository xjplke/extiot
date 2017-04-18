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
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/rakyll/ticktock"
	"github.com/rakyll/ticktock/t"
)

/*
Options:
 [-help]                      Display help
 [-a pub|sub]                 Action pub (publish) or sub (subscribe)
 [-m <message>]               Payload to send
 [-n <number>]                Number of messages to send or receive
 [-q 0|1|2]                   Quality of Service
 [-clean]                     CleanSession (true if -clean is present)
 [-id <clientid>]             CliendID
 [-user <user>]               User
 [-password <password>]       Password
 [-broker <uri>]              Broker URI
 [-topic <topic>]             Topic
 [-store <path>]              Store Directory

*/

//load from device config
type Device struct {
	ProductType string
	ProductId   string
	Sn          string
	Mac         string
	client      MQTT.Client
	deviceid    string
	datatopic   string
	configtopic string
	qos         int
}

func NewDevice(configfile string) *Device {
	f, err := ioutil.ReadFile(configfile)
	if err != nil {
		fmt.Printf("%s\n", err)
		panic(err)
	}
	fmt.Println(string(f))

	var obj interface{}
	err2 := json.Unmarshal([]byte(f), &obj)
	if err2 != nil {
		fmt.Println("unmarshal error!", err2)
		return nil
	}
	configmap := obj.(map[string]interface{})
	return &Device{
		ProductType: configmap["ProductType"].(string),
		ProductId:   configmap["ProductId"].(string),
		Sn:          configmap["SN"].(string),
		Mac:         configmap["MAC"].(string),
		deviceid:    configmap["ProductId"].(string) + "_" + configmap["SN"].(string),
		datatopic:   "extiot/device/data/" + configmap["ProductId"].(string) + "_" + configmap["SN"].(string),
		configtopic: "extiot/device/config/" + configmap["ProductId"].(string) + "_" + configmap["SN"].(string),
		qos:         1,
	}
}

func (d *Device) SetClient(client MQTT.Client) {
	d.client = client
}

func (j *Device) Run() error {
	fmt.Printf("ticked at %v\n", time.Now())
	//do get data
	data := make(map[string]interface{})
	data["temperature"] = 23
	data["time"] = time.Now()
	b, err := json.Marshal(data)
	if err != nil {
		fmt.Println("error:", err)
	}
	fmt.Println("publish device info ", string(b))
	token := j.client.Publish(j.datatopic, byte(j.qos), false, b)
	token.Wait()
	return nil
}

func main() {
	//topic := flag.String("topic", "extiot/device", "The topic name to/from which to publish/subscribe")
	broker := flag.String("broker", "tcp://127.0.0.1:1883", "The broker URI. ex: tcp://10.10.1.1:1883")
	password := flag.String("password", "", "The password (optional)")
	user := flag.String("user", "", "The User (optional)")
	//	clientid := flag.String("clientid", "device", "The ClientID (optional)")
	//deviceid := flag.String("deviceid", "device_xxx", "The DeviceID (optional)")
	config := flag.String("config", "./config.json", "device config, generate by platform")
	cleansess := flag.Bool("clean", false, "Set Clean Session (default false)")
	qos := flag.Int("qos", 1, "The Quality of Service 0,1,2 (default 0)")
	store := flag.String("store", ":memory:", "The Store Directory (default use memory store)")
	flag.Parse()

	//if *topic == "" {
	//	fmt.Println("Invalid setting for -topic, must not be empty")
	//	return
	//}

	device := NewDevice(*config)
	clientid := device.deviceid

	fmt.Printf("Sample Info:\n")
	fmt.Printf("\tbroker:    %s\n", *broker)
	fmt.Printf("\tclientid:  %s\n", clientid)
	fmt.Printf("\tdeviceid:  %s\n", device.deviceid)
	fmt.Printf("\tuser:      %s\n", *user)
	fmt.Printf("\tpassword:  %s\n", *password)
	fmt.Printf("\tqos:       %d\n", *qos)
	fmt.Printf("\tcleansess: %v\n", *cleansess)
	fmt.Printf("\tstore:     %s\n", *store)

	opts := MQTT.NewClientOptions()
	opts.AddBroker(*broker)
	opts.SetClientID(clientid)
	opts.SetUsername(*user)
	opts.SetPassword(*password)
	opts.SetCleanSession(*cleansess)
	if *store != ":memory:" {
		opts.SetStore(MQTT.NewFileStore(*store))
	}

	//init
	choke := make(chan [2]string)

	opts.SetDefaultPublishHandler(func(client MQTT.Client, msg MQTT.Message) {
		fmt.Println(string(msg.Payload()))
		choke <- [2]string{msg.Topic(), string(msg.Payload())}
	})

	client := MQTT.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}

	//sub
	fmt.Println("sub topic:", device.configtopic, "qos:", *qos)
	if token := client.Subscribe(device.configtopic, byte(*qos), nil); token.Wait() && token.Error() != nil {
		fmt.Println(token.Error())
		os.Exit(1)
	}

	device.SetClient(client)
	//pub
	//上报数据
	//如何避免设备恶意的平凡上报数据？
	//1、上报次数收费。
	//2、单个终端上报消息太平凡可以kick掉，并禁止在一定时间链接mqtt。
	//  这个跟核心业务没关系。以后再做
	err := ticktock.Schedule(
		"device_pub",
		device,
		&t.When{Every: t.Every(5).Seconds()})
	if err != nil {
		fmt.Println(err)
		return
	}
	go ticktock.Start()

	//process get message
	//接收配置
	for {
		incoming := <-choke
		fmt.Printf("RECEIVED TOPIC: %s MESSAGE: %s\n", incoming[0], incoming[1])
		var obj interface{}
		err2 := json.Unmarshal([]byte(incoming[1]), &obj)
		if err2 != nil {
			fmt.Println("unmarshal error!")
			continue
		}
		configmap := obj.(map[string]interface{})
		fmt.Println("configmap ", configmap)
		if interval, ok := configmap["interval"]; ok {
			fmt.Println("interval :", interval)
			ticktock.Cancel("device_pub")
			err := ticktock.Schedule(
				"device_pub",
				device,
				&t.When{Every: t.Every(int(interval.(float64))).Seconds()})
			if err != nil {
				fmt.Println(err)
				return
			}
		}
	}

	client.Disconnect(250)
}
