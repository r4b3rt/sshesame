package main

import (
	"encoding/base64"
	"fmt"
	"log"
	"net"

	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
)

type channelMetadata struct {
	channelID int
	conn      ssh.ConnMetadata
}

type tcpipChannelData struct {
	Address           string
	Port              uint32
	OriginatorAddress string
	OriginatorPort    uint32
}

func (data tcpipChannelData) String() string {
	return fmt.Sprintf("%v -> %v", net.JoinHostPort(data.OriginatorAddress, fmt.Sprint(data.OriginatorPort)), net.JoinHostPort(data.Address, fmt.Sprint(data.Port)))
}

func handleNewChannel(newChannel ssh.NewChannel, conn channelMetadata) {
	var channelData interface{}
	accept := true
	switch newChannel.ChannelType() {
	case "session":
	case "direct-tcpip":
		channelData = new(tcpipChannelData)
	default:
		log.Println("Unsupported channel type", newChannel.ChannelType())
		accept = false
	}
	channelDataString := ""
	if channelData != nil {
		if err := ssh.Unmarshal(newChannel.ExtraData(), channelData); err != nil {
			log.Println("Failed to parse channel data:", err)
			accept = false
		}

		channelDataString = fmt.Sprint(channelData)
	}
	if channelDataString == "" {
		channelDataString = base64.RawStdEncoding.EncodeToString(newChannel.ExtraData())
	}

	conn.getLogEntry().WithFields(logrus.Fields{
		"channel_type":       newChannel.ChannelType(),
		"channel_extra_data": channelDataString,
		"accepted":           accept,
	}).Infoln("New channel requested")

	if !accept {
		if err := newChannel.Reject(ssh.Prohibited, ""); err != nil {
			log.Println("Failed to reject new channel:", err)
		}
		return
	}

	channel, requests, err := newChannel.Accept()
	if err != nil {
		log.Println("Failed to accept new channel:", err)
		return
	}
	defer channel.Close()
	defer conn.getLogEntry().Infoln("Channel closed")

	go handleChannelRequests(requests, conn)

	channelInput := make(chan string)
	defer close(channelInput)

	go func() {
		for input := range channelInput {
			conn.getLogEntry().WithField("input", input).Infoln("Channel input received")
		}
	}()

	switch newChannel.ChannelType() {
	case "direct-tcpip":
		err = handleDirectTCPIPChannel(channel, channelData.(*tcpipChannelData).Port, channelInput)
	case "session":
		err = handleSessionChannel(channel, channelInput)
	default:
		log.Println("Unsupported channel type", newChannel.ChannelType())
		return
	}
	if err != nil {
		log.Println("Failed to read from channel:", err)
		return
	}
}
