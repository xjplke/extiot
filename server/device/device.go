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
	"strconv"
	"time"
	//"encoding/json"
	"flag"
	"fmt"
	"os"
	//"time"

	"github.com/bwmarrin/snowflake"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/xjplke/extiot"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
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

type deviceService struct {
	client MQTT.Client
	db     *mgo.Database
	node   *snowflake.Node
}

func (ds *deviceService) onMsgRegister(msg *extiot.Message) {
	fmt.Println("onMsgRegister ", msg.ToJson())
	db := ds.db
	node := ds.node
	client := ds.client

	//TODO check if ProductID/ProductType valid

	//found device in db
	deviceIns := extiot.DeviceInstance{}
	err := db.C("device").Find(bson.M{"product_id": msg.ProductID, "sn": msg.Sn}).One(&deviceIns)
	if err != nil { //not fond
		fmt.Println("device for ProductID", msg.ProductID, "sn", msg.Sn, err)
		deviceIns.DeviceID = node.Generate().Int64()
		deviceIns.Device = msg.Device
		deviceIns.CreateAt = time.Now()
		db.C("device").Insert(deviceIns)

		//TODO get default configs
	}

	msgAck := extiot.NewMessage(deviceIns.Device, deviceIns.DeviceID, extiot.MsgRegisterAck, deviceIns.CurrentConfigs)
	msgAck.DeviceID = deviceIns.DeviceID
	//return device in MessageRegisterACK
	client.Publish("extiot/device/config/"+strconv.FormatInt(deviceIns.ProductID, 10)+"_"+deviceIns.Sn, byte(1), false, msgAck.ToJson())
	return
}

func (ds *deviceService) onMsgStatus(msg *extiot.Message) {
	fmt.Println("onMsgStatus ", msg.ToJson())
	return
}

func (ds *deviceService) onMsgDisconnect(msg *extiot.Message) {
	fmt.Println("onMsgDisconnect ", msg.ToJson())
	return
}

func main() {
	broker := flag.String("broker", "tcp://127.0.0.1:1883", "The broker URI. ex: tcp://10.10.1.1:1883")
	password := flag.String("password", "manager", "The password (optional)")
	user := flag.String("user", "system", "The User (optional)")
	id := flag.String("id", "deviceservice", "The ClientID (optional)")
	//cleansess := flag.Bool("clean", false, "Set Clean Session (default false)")
	//qos := flag.Int("qos", 1, "The Quality of Service 0,1,2 (default 0)")
	store := flag.String("store", ":memory:", "The Store Directory (default use memory store)")
	dburl := flag.String("dburl", "127.0.0.1:32769", "The mongo db url")
	dbname := flag.String("dbname", "extiot", "The mongo db name")
	flag.Parse()

	topic := "extiot/device/status/#"
	qos := 1
	cleansess := false
	machine := int64(1)
	//collection := "device"

	if topic == "" {
		fmt.Println("Invalid setting for -topic, must not be empty")
		return
	}

	fmt.Printf("Sample Info:\n")
	fmt.Printf("\tbroker:    %s\n", *broker)
	fmt.Printf("\tclientid:  %s\n", *id)
	fmt.Printf("\tuser:      %s\n", *user)
	fmt.Printf("\tpassword:  %s\n", *password)
	fmt.Printf("\ttopic:     %s\n", topic)
	fmt.Printf("\tqos:       %d\n", qos)
	fmt.Printf("\tcleansess: %v\n", cleansess)
	fmt.Printf("\tstore:     %s\n", *store)
	//connect database
	session, err := mgo.Dial(*dburl) //连接数据库
	if err != nil {
		panic(err)
	}
	defer session.Close()
	session.SetMode(mgo.Monotonic, true)

	db := session.DB(*dbname) //数据库名称
	//colldevice := db.C("device") //如果该集合已经存在的话，则直接返回

	opts := MQTT.NewClientOptions()
	opts.AddBroker(*broker)
	opts.SetClientID(*id)
	opts.SetUsername(*user)
	opts.SetPassword(*password)
	opts.SetCleanSession(cleansess)
	if *store != ":memory:" {
		opts.SetStore(MQTT.NewFileStore(*store))
	}

	//init
	choke := make(chan [2]string)
	opts.SetDefaultPublishHandler(func(client MQTT.Client, msg MQTT.Message) {
		choke <- [2]string{msg.Topic(), string(msg.Payload())}
	})

	client := MQTT.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		fmt.Println("xxxxx\n")
		panic(token.Error())
	}

	//snowflake
	node, err2 := snowflake.NewNode(machine)
	if err2 != nil {
		panic(err2)
	}
	ds := deviceService{
		client: client,
		db:     db,
		node:   node,
	}
	//sub
	if token := client.Subscribe(topic, byte(qos), nil); token.Wait() && token.Error() != nil {
		fmt.Println(token.Error())
		os.Exit(1)
	}

	for {
		incoming := <-choke
		fmt.Printf("RECEIVED TOPIC: %s MESSAGE: %s\n", incoming[0], incoming[1])

		//json to map
		msg := extiot.MsgFromJson(incoming[1])
		if msg == nil {
			continue
		}
		//设备上的deviceid怎么生成？！设备自己的sn号？貌似不好，怎么保证唯一性？
		//
		//必须和硬件保持一定关系：producttype_mac
		//save to mongodb.
		//是否需要根据Product定义的数据结构对数据进行验证？
		//数据版本的验证！？

		switch msg.MsgType {
		case extiot.MsgRegister:
			ds.onMsgRegister(msg)
		case extiot.MsgStatus:
			ds.onMsgStatus(msg)
		case extiot.MsgDisconnect:
			ds.onMsgDisconnect(msg)
		case extiot.MsgConfig:
			fallthrough
		case extiot.MsgRegisterAck:
			fallthrough
		default:
			fmt.Println("shold not get this msg at device service", msg.ToJson())
		}
	}

	client.Disconnect(250)

}
