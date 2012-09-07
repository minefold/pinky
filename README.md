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

    opped               { actor_playername: 'whatupdave',
                          target_playername: 'chrislloyd' }
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


