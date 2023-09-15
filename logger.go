package main

import (
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func init() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
}

type RoomIPLogger struct {
	zerolog zerolog.Logger
}

func GetRoomIPLogger(ip string, roomCode string) RoomIPLogger {
	return RoomIPLogger{log.With().Str("ip", ip).Str("room-code", roomCode).Logger()}
}

func (l RoomIPLogger) RemovingRoom() {
	l.zerolog.Info().Msg("Removing room")
}

func (l RoomIPLogger) HostDisconnected() {
	l.zerolog.Info().Msg("Host disconnected")
}

func (l RoomIPLogger) JoinedRoom() {
	l.zerolog.Info().Msg("Joined room")
}

func (l RoomIPLogger) LeftRoom() {
	l.zerolog.Info().Msg("Left room")
}

func LogCreatedRoom(roomCode string) {
	log.Info().Str("room-code", roomCode).Msg("Created")
}

func LogReconnectedToRoom(roomCode string) {
	log.Info().Str("room-code", roomCode).Msg("Reconnected")
}

func LogStartedServer(port string) {
	log.Info().Msgf("Starting server on port %v", port)
}

func LogErrorWhileUpgradingHTTP(err error) {
	log.Error().Err(err).Msg("Error while upgrading HTTP")
}
