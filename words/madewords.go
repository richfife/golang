// madewords.go - A program to build a trie from words in a file and scan for
// matches using multiple worker goroutines.
//
// Usage:  madewords <file name> [<worker thread count>]
//
// Implementation of a coding challenge using a trie and multiple worker
// goroutines.
//
// Read a file containing words and build a trie from them.
// Then, scan the words in the file to find words that can be made by
// concatenating two or more shorter words from the list.
// Output the count of words that can be made this way and also output the
// two longest constructable words.
//
// I implemented this using a trie built around each letter, but a more
// efficient approach would be to use a hash map.  We are, with this solution,
// using each char in a similar way to a hash value, so we are saving time on
// constructing hash values at the expense of dealing with a deeper, unbalanced
// search trie.
//
// The emphasis was speed over simplicity.

package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

type trieNode struct {
	end_of_word bool
	children    [32]*trieNode
}

type sliceWorker struct {
	// Set of strings to be added to the trie or checked for matches
	// for this context
	slice []string

	// These are specific to this subset.  They'll be checked
	// against the other subsets at the end.
	longest_word   string
	second_longest string

	// Matches in this subset.  They will be all added up at the end.
	match_count int
}

type WorkerTask int

type WorkRequest struct {
	task   WorkerTask
	worker *sliceWorker
}

// Tasks we can require the workeur goroutines to perform.
const (
	build_trie WorkerTask = iota
	scan_words
)

func main() {
	var top trieNode

	worker_count := 8
	words_file := ""

	switch len(os.Args) {
	case 3:
		var err error
		worker_count, err = strconv.Atoi(os.Args[2])
		if err != nil {
			log.Fatal(err)
		}
		fallthrough
	case 2:
		words_file = os.Args[1]
	case 1:
		words_file = "golang/words/wordsforproblem.txt"
	default:
		log.Fatal("Format:  main <file name> [<worker thread count>]")
	}

	fmt.Println("Threads:", worker_count)

	// Read the entire file into a memory buffer
	file, err := os.Open(words_file)

	if err != nil {
		log.Fatal(err)
	}

	defer file.Close()

	stb, err := file.Stat()

	if err != nil {
		log.Fatal(err)
	}

	bytes := make([]byte, stb.Size())

	_, err = file.Read(bytes)

	if err != nil {
		log.Fatal(err)
	}

	// Seed the top entries of the trie.  Having the topmost part of
	// the trie ready helps to avoid the need for synchronization in
	// the goroutines later.
	for i := range top.children {
		top.children[i] = &trieNode{}
	}

	words := strings.Fields(string(bytes))

	// Channels to request work and notify of completion.
	work_required := make(chan WorkRequest, worker_count)
	work_completed := make(chan (*sliceWorker), worker_count)

	// Worker contexts
	workers := make([]sliceWorker, worker_count)

	// The first worker starts at byte 0
	slice_start := 0

	// Build the worker contexts
	for i := range workers {
		// What is the last word this worker should handle?
		slice_stop := (int)((len(words) * (i + 1)) / worker_count)

		workers[i].slice = words[slice_start:slice_stop]

		// The next worker starts where this one stops
		slice_start = slice_stop
	}

	// Launch goroutines pool
	for range workers {
		// Start a goroutine for each worker.
		go func() {
			for request := range work_required {
				switch request.task {

				// Task 1:  Build the trie with the words we have
				case build_trie:
					for _, one_string := range request.worker.slice {
						current_node := &top

						for _, one_char := range one_string {
							if current_node.children[one_char&0x1F] == nil {
								current_node.children[one_char&0x1F] = &trieNode{}
							}

							current_node = current_node.children[one_char&0x1F]
						}

						current_node.end_of_word = true
					}

					// Task 2:  Scan the words against the trie
				case scan_words:
					var check_stack [64]string

					for _, one_string := range request.worker.slice {
						current_node := &top
						check_stack_index := 0

						for index, one_char := range one_string {
							current_node = current_node.children[one_char&0x1F]

							if current_node.end_of_word {
								check_stack[check_stack_index] = one_string[index+1:]
								check_stack_index++
							}
						}

					NEXT_SUBMATCH:
						for check_stack_index > 0 {
							check_stack_index--

							current_node := &top
							one_sub := check_stack[check_stack_index]

							for index, one_char := range one_sub {
								current_node = current_node.children[one_char&0x1F]

								// This path didn't lead to a match.
								if current_node == nil {
									continue NEXT_SUBMATCH
								}

								if current_node.end_of_word {
									check_stack[check_stack_index] = one_sub[index+1:]
									check_stack_index++
								}
							}

							// We got to the end of the string.  Is it a word?
							if current_node.end_of_word {
								// It is.  Match!
								request.worker.match_count++

								// Is it a new longest or second longest?
								if len(one_string) > len(request.worker.second_longest) {
									if len(one_string) > len(request.worker.longest_word) {
										request.worker.second_longest = request.worker.longest_word
										request.worker.longest_word = one_string
									} else {
										request.worker.second_longest = one_string
									}
								}
								break
							}
						}
					}
				}

				// We've completed the task.  Tell the main thread.
				work_completed <- request.worker
			}

			// We're about to exit because the work_required channel was closed.
			work_completed <- nil
		}()
	}

	// OK everyone.  Build the trie!
	for i := range workers {
		work_required <- WorkRequest{build_trie, &workers[i]}
	}

	// Wait until everyone is done.
	for range workers {
		<-work_completed
	}

	// The topmost trie node gets marked as a word if the
	// source file has windows line endings.  Clear that.
	top.end_of_word = false

	// OK everyone.  Scan for matches!
	for i := range workers {
		work_required <- WorkRequest{scan_words, &workers[i]}
	}

	// Set up result storage while the goroutines are ticking
	match_count := 0

	var longest_word string
	var second_longest string

	// Wait for tasks to complete
	for range workers {
		worker := <-work_completed

		// Add up the results
		match_count += worker.match_count

		if len(worker.longest_word) > len(longest_word) {
			second_longest = longest_word
			longest_word = worker.longest_word
		} else if len(worker.longest_word) > len(second_longest) {
			second_longest = worker.longest_word
		}

		if len(worker.second_longest) > len(second_longest) {
			second_longest = worker.second_longest
		}
	}

	// No more work is required.
	close(work_required)

	// Wait for the goroutines to exit (not strictly necessary).
	for range workers {
		<-work_completed
	}

	// No more workers = no more need to signal job completion.
	close(work_completed)

	fmt.Println(match_count)
	fmt.Println("Longest: ", string(longest_word))
	fmt.Println("Second : ", string(second_longest))
}
