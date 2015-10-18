package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"strings"
	"time"

	"github.com/jeffrydegrande/kongo"
	"github.com/samalba/dockerclient"
)

var (
	docker     *dockerclient.DockerClient
	kong       *kongo.Kong
	config     map[string]interface{}
	kongUrl    string
	dockerSock string
)

func init() {
	flag.StringVar(&kongUrl, "k", "http://localhost:8001", "url of kong instance")
	flag.StringVar(&dockerSock, "s", "/var/run/docker.sock", "path of the docker UNIX socket")

	configFile, err := ioutil.ReadFile("config.json")
	if err != nil {
		panic(err)
	}

	err = json.Unmarshal(configFile, &config)
	if err != nil {
		panic(err)
	}

}

func checkAndSync(containerId string) {
	info, _ := docker.InspectContainer(containerId)

	isMonitored := false
	service := ""
	plugins := make([]string, 1)

	for _, env := range info.Config.Env {
		parts := strings.Split(env, "=")
		if parts[0] == "KONG_SERVICE" {
			isMonitored = true
			service = parts[1]
		}
		if parts[0] == "KONG_PLUGINS" {
			plugins = strings.Split(parts[1], ",")
		}
	}

	if !isMonitored {
		return
	}

	log.Printf("Registering container: %s %s %s %#v\n", info.Name, info.NetworkSettings.IPAddress, service, plugins)

	path := fmt.Sprintf("/%s", service)
	targetUrl := fmt.Sprintf("http://%s:8080", info.NetworkSettings.IPAddress)

	endpoint := kongo.NewEndpoint(service, path, targetUrl)
	endpoint.PreserveHost = config["api"].(map[string]interface{})["preserve_host"].(bool)
	endpoint.StripPath = config["api"].(map[string]interface{})["strip_path"].(bool)

	err := kong.SetEndpoint(endpoint)

	if err != nil {
		log.Println(err)
	}

	/*
	   for _, plugin := range plugins {
	           log.Printf("using config: %#v\n", config[plugin])
	   }
	*/
}

func eventCallback(event *dockerclient.Event, ec chan error, args ...interface{}) {
	if event.Status != "start" {
		return
	}
	checkAndSync(event.Id)
}

func main() {
	flag.Parse()
	docker, _ = dockerclient.NewDockerClient("unix:///var/run/docker.sock", nil)
	kong = kongo.NewKong(kongUrl)

	endpoints, err := kong.GetEndpoints()
	if err != nil {
		panic(err)
	}
	for _, endpoint := range endpoints {
		log.Printf("%s => %s\n", endpoint.Path, endpoint.TargetUrl)
	}

	// Get running containers
	containers, err := docker.ListContainers(false, false, "")
	if err != nil {
		log.Fatal(err)
	}

	for _, c := range containers {
		checkAndSync(c.Id)
	}

	// Listen to events
	docker.StartMonitorEvents(eventCallback, nil)

	// Hold the execution to look at the events coming
	for true {
		time.Sleep(3600 * time.Second)
		log.Println("PONG")
	}
}
