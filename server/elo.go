package main

import "math"

const (
	startingRating = 1200
	kFactorNew     = 40
	kFactorDefault = 20
	newPlayerGames = 30
)

func kFactor(gamesPlayed int) float64 {
	if gamesPlayed < newPlayerGames {
		return kFactorNew
	}
	return kFactorDefault
}

func expectedScore(ratingA, ratingB int) float64 {
	return 1.0 / (1.0 + math.Pow(10, float64(ratingB-ratingA)/400.0))
}

type ELOUpdate struct {
	PlayerID   string
	OldRating  int
	NewRating  int
	GamesAfter int
}

func CalculateELO(winnerRating, loserRating, winnerGames, loserGames int, draw bool) (winnerNew, loserNew int) {
	eWin := expectedScore(winnerRating, loserRating)
	eLose := expectedScore(loserRating, winnerRating)

	kWin := kFactor(winnerGames)
	kLose := kFactor(loserGames)

	var sWin, sLose float64
	if draw {
		sWin = 0.5
		sLose = 0.5
	} else {
		sWin = 1.0
		sLose = 0.0
	}

	winnerNew = winnerRating + int(math.Round(kWin*(sWin-eWin)))
	loserNew = loserRating + int(math.Round(kLose*(sLose-eLose)))
	return
}
