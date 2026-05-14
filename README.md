# YouTube Synchronizer Backend

Backend server for the YouTube Synchronizer extension, built with Go. This server facilitates real-time synchronization of YouTube video playback (play, pause, time, playback rate, and video path) between a host and multiple clients.

## Features
- **WebSocket Connection**: Used by the host to broadcast their current playback state in real-time.
- **Server-Sent Events (SSE)**: Used by clients to receive real-time playback updates from the host with low overhead.
- **Room Management**: Hosts create sessions (rooms) identified by unique, randomly generated codes (typically 6-digit codes used in the extension).
- **Host Reconnection**: JWT-based reconnection mechanism allows the host to momentarily disconnect and reconnect without dropping connected clients or losing the room's state.
- **State Prediction**: The server estimates the current video time based on the last update, playback rate, and elapsed time, ensuring newly joined clients sync instantly.
- **Rate Limiting & CORS**: Built-in HTTP rate limiting and CORS configuration to restrict origins and protect against abuse.

## Endpoints

### `GET /ws` (WebSocket)
Used by the host to connect, create a room, and send state updates.
- **Query Params**: `?reconnectKey=<jwt_token>` (optional, to reconnect to an existing room as a host).
- **Messages received by server**: JSON objects with `type` (e.g., `sync`, `startPlaying`, `pause`, `pathChange`, `rateChange`, `removeRoom`) and the corresponding data (`path`, `time`, `rate`, `isPaused`).
- **Messages sent by server**: 
  - Room code: `{"type": "code", "code": "..."}`
  - Reconnect keys (sent every minute): `{"type": "reconnectKey", "key": "..."}`

### `GET /room/{roomCode}` (Server-Sent Events)
Used by clients to connect to an existing room and listen for sync events.
- Emits the initial video state (with predicted time) upon successful connection.
- Relays real-time messages from the host to all connected clients.
- Emits `hostDisconnected` and `hostReconnected` events to notify clients when the host's connection drops and recovers.

### `GET /room/{roomCode}/path`
Returns the current YouTube video path for a specific room. Useful for clients to check what video is currently playing before fully syncing.

## Setup & Running

### Prerequisites
- Go 1.19+

### Installation

1. **Clone the repository:**
   ```bash
   git clone https://github.com/Artiu/youtube-synchronizer-backend.git
   cd youtube-synchronizer-backend
   ```

2. **Configure environment variables:**
   Copy the example environment file:
   ```bash
   cp .env.example .env
   ```
   Open `.env` and set your variables:
   ```env
   PORT=3000
   JWT_SECRET=your_super_secret_jwt_key
   ```

3. **Install dependencies:**
   ```bash
   go mod download
   ```

4. **Run the server:**
   ```bash
   go run main.go
   ```

## Architecture Notes
- The server stores the state of each room (path, time, playback rate, pause status) in memory.
- Uses `sync.RWMutex` to ensure thread-safe access to room state and connection maps.
- Disconnected hosts have a 2-minute window to reconnect using their valid JWT `reconnectKey` before the room is permanently closed and clients are disconnected.

## Key Dependencies
- [chi](https://github.com/go-chi/chi) - Lightweight router
- [gobwas/ws](https://github.com/gobwas/ws) - High-performance WebSocket implementation
- [zerolog](https://github.com/rs/zerolog) - Fast JSON logging
- [golang-jwt](https://github.com/golang-jwt/jwt) - JWT generation and validation
