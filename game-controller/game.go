package main

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
)

const (
	initialEnergy      = 100
	initialLife        = 100
	initialTemperature = 0
	initialRadiation   = 500
	initialSolarFlare  = false
)

// GameInfo contains information about the state of the game
type GameInfo struct {
	// Running defines whether the game is running or not
	Running    bool      `json:"running"`
	StartedAt  time.Time `json:"startedAt"`
	Reading    Reading   `json:"readings"`
	Teams      []Team    `json:"teams"`
	adminToken string
	start      chan TokenRequest
	stop       chan TokenRequest
	join       chan JoinRequest
	exit       chan []byte
	shield     chan ShieldRequest
}

// Team contains information about a team
type Team struct {
	Name   string `json:"name"`
	token  string
	Energy int64 `json:"energy"`
	Life   int64 `json:"life"`
	Shield bool  `json:"shield"`
}

var game = GameInfo{
	Running: false,
	start:   make(chan TokenRequest),
	stop:    make(chan TokenRequest),
	join:    make(chan JoinRequest),
	shield:  make(chan ShieldRequest),
	exit:    make(chan []byte),
	Reading: Reading{SolarFlare: initialSolarFlare, Temperature: initialTemperature, Radiation: initialRadiation},
	Teams:   []Team{},
}

func (game *GameInfo) run(adminToken string) {
	game.adminToken = adminToken
	var wg sync.WaitGroup
	ticker := time.NewTicker(time.Second * 1)
	for {
		select {
		case req := <-game.start:
			success, message := game.startGame(req.token)
			if success {
				go game.getReadings(&wg)
				go game.runEngine(&wg)
			}
			req.Response <- GameResponse{success: success, message: message}
			close(req.Response)
		case req := <-game.stop:
			success, message := game.stopGame(req.token)
			if success {
				wg.Wait()
			}
			req.Response <- GameResponse{success: success, message: message}
			close(req.Response)
		case req := <-game.join:
			success, message := game.joinGame(req.name)
			req.Response <- GameResponse{success: success, message: message}
			close(req.Response)
		case req := <-game.shield:
			success, message := game.enableShield(req.token, req.enable)
			req.Response <- GameResponse{success: success, message: message}
			close(req.Response)
		case <-ticker.C:
			if game.Running && game.isOver() {
				log.Println("Game is over. Only one team is left.")
				game.Running = false
				wg.Wait()
			}

			m, err := json.Marshal(&game)
			if err != nil {
				log.Println("Error parsing to JSON.", err)
				continue
			}
			log.Println(string(m))
			h.broadcast <- []byte(m)

		case <-game.exit:
			log.Println("Exiting game")
			return
		}
	}
}

func (game *GameInfo) stopGame(token string) (bool, string) {
	if !game.authorizeAdmin(token) {
		log.Printf("Unauthorized request to stop game. Token: %s\n", token)
		return false, "Unauthorized"
	}
	if game.Running {
		game.Running = false
		log.Println("Game stopped!")
		return true, "Game stopped"
	}
	log.Println("Game is already stopped, not doing anything...")
	return false, "Game is already stopped, not doing anything"
}

func (game *GameInfo) startGame(token string) (bool, string) {
	if !game.authorizeAdmin(token) {
		log.Printf("Unauthorized request to start game. Token: %s\n", token)
		return false, "Unauthorized"
	}
	if game.Running {
		log.Println("Game is already started, not doing anything...")
		return false, "Game is already started, not doing anything"
	}

	if len(game.Teams) < 2 {
		log.Println("At least 2 players are needed to start the game")
		return false, "At least 2 players are needed to start the game"
	}

	game.Running = true
	game.StartedAt = time.Now()
	log.Println("Game started!")
	return true, "Game started"
}

func (game *GameInfo) joinGame(name string) (bool, string) {
	if game.Running {
		message := fmt.Sprintf("Team '%s' cannot join the game because it's already running", name)
		log.Println(message)
		return false, message
	}
	if game.teamExists(name) {
		message := fmt.Sprintf("Team '%s' already exists.", name)
		log.Println(message)
		return false, message
	}
	team := Team{Name: name, Life: initialLife, Energy: initialEnergy, Shield: false, token: randToken()}
	game.Teams = append(game.Teams, team)
	log.Printf("Team '%s' joined the game", name)
	return true, team.token
}

func (game *GameInfo) enableShield(token string, enable bool) (bool, string) {
	i, ok := game.authorizeTeam(token)
	if !ok {
		log.Printf("Invalid token '%s'\n", token)
		return false, "Unauthorized"
	}

	game.Teams[i].Shield = enable
	message := fmt.Sprintf("Team '%s' set shield to %t", game.Teams[i].Name, game.Teams[i].Shield)
	log.Println(message)
	return true, message
}

func (game *GameInfo) getReadings(wg *sync.WaitGroup) {
	wg.Add(3)
	go solarFlareRoutine(wg, game)
	go temperatureRoutine(wg, game)
	radiationRoutine(wg, game)
}
