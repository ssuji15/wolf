VM_NAME = myvm
SSH = multipass exec $(VM_NAME) -- 

.PHONY: create  transfer bootstrap init

init: create transfer bootstrap
	@echo "Init success!"

create:
	multipass info $(VM_NAME) >/dev/null 2>&1 || \
	multipass launch --name $(VM_NAME) --mem 16G --disk 50G --cpus 10

transfer: 
	multipass transfer -r ../wolf $(VM_NAME):/home/ubuntu
	multipass transfer -r ../wolf-worker $(VM_NAME):/home/ubuntu
	multipass transfer startup.sh $(VM_NAME):/home/ubuntu
	multipass transfer tables.sql $(VM_NAME):/home/ubuntu
	$(SSH) sudo mv /home/ubuntu/wolf /root/
	$(SSH) sudo mv /home/ubuntu/wolf-worker /root/
	$(SSH) sudo mv /home/ubuntu/startup.sh /root/
	$(SSH) sudo mv /home/ubuntu/tables.sql /root/

bootstrap:
	$(SSH) sudo chmod +x /root/startup.sh
	$(SSH) sudo bash /root/startup.sh

destroy:
	multipass delete $(VM_NAME)
	multipass purge