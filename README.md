# Pinky

Pinky is a daemon process that starts, monitors and stops servers on a single instance.

## Usage

pinky <box-id>

## Required ENV

AWS_ACCESS_KEY=DLHDLKJHDLKJHDLKH
AWS_SECRET_KEY=DSLKSJDHFLDKJHFLDSKJHFDSLKJHDLS
MONGO_URL=mongodb://10.0.2.2:27017/partycloud
REDIS_URL=tcp:10.0.2.2:6379

## How it works

brpop /box/1/queue
  - type (start|stop|broadcast|tell)

### Ignored commands
* While stopping or starting ignore commands
* While running or stopped execute commands

### START
  - server_id
  - server settings
  - max ram

Example:

{
  "name"=>"start",
  "serverId"=>"1234",
  "funpack"=>"https://minefold-development.s3.amazonaws.com/funpacks/dummy/1.tar.lzo",
  "ram"=>{"min"=>1024, "max"=>1024},
  "world"=>"https://minefold-development.s3.amazonaws.com/worlds/501c6845c3b5a06485000002/world-data.incremental.tar.lzo",
  "settings"=>{
    "banned"=>["atnan"],
    "game_mode"=>1,
    "new_player_can_build"=>false,
    "ops"=>["chrislloyd"],
    "seed"=>123456789,
    "spawn_animals"=>true,
    "spawn_monsters"=>true,
    "whitelisted"=>["whatupdave"]
  }
}


### STOP
  - server_id

### BROADCAST
  - server_id
  - message

### TELL
  - server_id
  - user_id
  - message
  
## Paths

$SERVERS (typically /mnt/servers)

$SERVERS/1234.pid
$SERVERS/1234/pipe_stdin
$SERVERS/1234/pipe_stdout

$SERVERS/1234/funpack
$SERVERS/1234/working   (this will only exist for persistent state games)
$SERVERS/1234/backup    (ditto)

## Starting a server
* determine a port       ( process TBD )
* download the funpack
* call prepare
* call start
    - kill if:
      - exceptions
      - if there's been too long between stdout lines
      - server ready not received in timeout
* monitor

## Monitoring a server
* Run a backup every x mins if backups are required
* Notify process exit
* Watch for stdout lines (should be JSON)

.
    
    event=started at=234897
    { at: ISO-6601, event: started, ... }
    
    
    EVENT TYPES
      started
      stopping
      player_connected      { playername: '..' }
      player_disconnected
      chatted
      * connected_players
    
      settings_changed { type: 'opped', actor_playername: 'whatupdave',
                            target_playername: 'chrislloyd'
        opped
        deopped
        whitelist_added
        whitelist_removed
        banned
        pardoned
    
      info          { event: info, msg: 'bukkit outta date yall'}
      warning       { event: warning, type: 'stressed' }
      critical      { event: critical,
                      type: 'unknown',
                      msg: 'Java.Bullshit.Error occurred on line 69' }

* on these events:
  - LPUSH server:events
  - pinky notifies Brain (Resque) (Brain calls webhooks)
  - pinky updates state of servers and box

## Stopping a server
* set server:<server-id>:state to "stopping"
* Stop the process
  - send stop command
  - wait 30 seconds
  - kill process if still alive
* Run the backup if necessary
* remove server artifacts
  - del server:<server-id>:state
  - remove pid file
  - remove server path

## Deployment

* send stop command to pinky
* box transitions to stopping state which ignores every command
* waits for internal jobs to finish (chris can use semaphores, we will use fancy GO things)
* exit

* (upstart restarts the process)

## Redis

    pinky:1:state (up|down) # when down jobs are ignored

    pinky:1:servers:1234 {
      "pid"  => 2418,
      "port" => 4083
    }

    server:1234:state (starting|up|down)

    pinky:1:heartbeat # Updated every 10 seconds with 30 second timeout
    

## Example Jobs

    lpush pinky:1:in "{\"name\":\"start\",\"serverId\":\"508227b5474a80599bcab3aa\",\"funpack\":\"https://minefold-development.s3.amazonaws.com/funpacks/dummy/1.tar.lzo\",\"ram\": { \"min\": 1024, \"max\": 1024  },\"world\":\"https://minefold-development.s3.amazonaws.com/worlds/1234/1234.1350676908.tar.lzo\", \"settings\" : { \"banned\": [\"atnan\"], \"game_mode\": 1, \"new_player_can_build\" : false,\"ops\": [\"chrislloyd\"],\"seed\": 123456789,\"spawn_animals\": true,    \"spawn_monsters\": true,\"whitelisted\": [\"whatupdave\"]  }}"
    lpush pinky:1:in "{\"name\":\"stop\",\"serverId\":\"508227b5474a80599bcab3aa\"}"

## TODO

log application crashes with bugsnag
support BROADCAST job
support TELL job
push job accept status (accepted|ignored)
push job complete status (succeeded|failed)
support TCP/UDP port ranges
recover backups not working (s3 down?)

¯\_(ツ)_/¯