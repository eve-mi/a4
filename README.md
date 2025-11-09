# a4

From the root directory, run :
- go run ./RA_algo.go --id=1 --port=5001 --peers=localhost:5001,localhost:5002,localhost:5003
- go run ./RA_algo.go --id=2 --port=5002 --peers=localhost:5001,localhost:5002,localhost:5003
- go run ./RA_algo.go --id=3 --port=5003 --peers=localhost:5001,localhost:5002,localhost:5003
to have the 3 nodes running.
NB: to add nodes, add 'localhost:5004' in the peers list

Once all the nodes are running, press 'enter' from the node you desire to see in the critical section. Repeat until satisfied.
