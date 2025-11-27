install:V:
	go build -o $HOME/bin/Denote .
	go build -o $HOME/bin/Drn cmd/Drn/main.go
	cp scripts/Djournal $HOME/bin/Djournal
	cp scripts/Dmerge $HOME/bin/Dmerge
	cp scripts/Dbkp $HOME/bin/Dbkp

clean:V:
	rm -f $HOME/bin/Denote $HOME/bin/Drn $HOME/bin/Djournal $HOME/bin/Dmerge $HOME/bin/Dbkp