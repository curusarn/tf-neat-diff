
build:
	go build -o tf-neat-diff cmd/main.go

install: build
	chmod u+x tf-neat-diff
	# Now copy the binary somewhere into your PATH - e.g.:
	@echo 'cp tf-neat-diff ~/bin/'
	#
	#
	### Intended usage ###
	# 1)
	@echo 'terragrunt plan -out /tmp/tf-plan | tf-neat-diff'
	# 2)
	@echo 'terragrunt apply /tmp/tf-plan'


