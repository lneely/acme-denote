install:V:
	go build -o $HOME/bin/Denote .
	go build -o $HOME/bin/Drn cmd/Drn/main.go
	cp scripts/Djournal $HOME/bin/Djournal
	cp scripts/Dmerge $HOME/bin/Dmerge

clean:V:
	rm -f $HOME/bin/Denote $HOME/bin/Drn $HOME/bin/Djournal $HOME/bin/Dmerge