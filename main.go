package pinky

import (
  	"fmt"
	"log"
  	"os"
  	"github.com/alphazero/Go-Redis"
)

func popRedisQueue(c chan string, client redis.Client, queue string) {
	for {
		bytes, e := client.Brpop(queue, 20);
		if e!= nil { log.Println ("error on BRPOP", e); return }
		
		// If the pop times out, it just returns the key, no value
		if len(bytes) > 1 {
			c <- fmt.Sprintf("%s", bytes[1])
		}
	}
}

func startWorld() {
	fmt.Println("Starting world")
}

func stopWorld() {
	fmt.Println("Stopping world")
}

func main() {
	boxId := os.Args[1]

	fmt.Println("box_id: ", boxId)

	spec := redis.DefaultSpec().Db(13)
	client, e := redis.NewSynchClientWithSpec (spec);
	if e != nil { log.Println ("failed to create the client", e); return }

	boxQueueKey := fmt.Sprintf("box/%s/queue", boxId)
	
	jobChannel := make(chan string)

	go popRedisQueue(jobChannel, client, boxQueueKey)
	
	for {
		job := <- jobChannel
		fmt.Println(job)
	
		switch job {
		case "start": startWorld()
		case "stop": stopWorld()
		default: fmt.Println("Unknown job", job)
		}
	}
	

  /*
    figure out what the current state is
    ie. find out what servers are currently running, update state, monitor etc.
  */

  /*
    process redis queue
  */



}