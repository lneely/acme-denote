install:V:
	go build -o $HOME/bin/Denote .
	cp scripts/Journal $HOME/bin/Journal

clean:V:
	rm -f $HOME/bin/Denote $HOME/bin/Journal