# Pinky

Pinky is a daemon process that starts, monitors and stops servers on a single instance.

## Usage

pinky <box-id>

## How it works

brpop /box/1/queue
  - type (start|stop|broadcast|tell|multi)

### State

box states:
box/1/state
  - starting
  - up
  - stopping
  - stopped

server states:
box/1/server/1/state
  - starting
  - up
  - stopping
  - stopped

box/1/server/1/queue

### Ignored commands
* While stopping or starting ignore commands
* While running or stopped execute commands

### START
  - server_id
  - server settings
  - max ram
  - # of server slots

### STOP
  - server_id

### BROADCAST
  - server_id
  - message

### TELL
  - server_id
  - user_id
  - message

### MULTI
  - (array of jobs)
  
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

  event=started at=234897

  { at: ISO-6601!!!!, event: started, ... }

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
  - pinky notifies Brain (Resque) (Brain calls webhooks)
  - pinky updates state of servers and box

## Stopping a server
* Stop the process (send stop command to the server)
* If the process doesn't stop, kill it (30 sec)
* Run the backup if necessary

## Deployment

* send stop command to pinky
* box transitions to stopping state which ignores every command
* waits for internal jobs to finish (chris can use semaphores, we will use fancy GO things)
* exit

* (upstart restarts the process)

## Redis state
pinky/1/state up # or down, meaning start jobs will be ignored
pinky/1/servers/1234 { # HASH
  "pid"  => 2418,
  "port" => 4083,
  "state" => "up"
}

* Updated every 10 seconds:
pinky/1/resources { # HASH
  "disk" => {
    "/dev/sda1" => { "used" => 1639, "total" => 8156 },
    "/dev/md0"  => { "used" => 8553, "total" => 3438696 }
  },
  "ram" => { "used" => 1024, "total" => "4096" },
}

## Example Jobs
lpush jobs/1 "{\"name\":\"start\",\"serverId\":\"1234\",\"funpack\":\"minecraft-essentials\",\"ram\": { \"min\": 1024, \"max\": 1024  }, \"settings\" : { \"banned\": [\"atnan\"], \"game_mode\": 1, \"new_player_can_build\" : false,\"ops\": [\"chrislloyd\"],\"seed\": 123456789,\"spawn_animals\": true,    \"spawn_monsters\": true,\"whitelisted\": [\"whatupdave\"]  }}"
lpush jobs/1 "{\"name\":\"stop\",\"serverId\":\"1234\"}"

## TODO

download funpack from s3
log application crashes with bugsnag
port allocation