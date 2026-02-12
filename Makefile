BINARY = regieleki
INSTALL_DIR = /usr/local/bin
DATA_DIR = /var/lib/regieleki

.PHONY: build clean install uninstall

build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BINARY) .

clean:
	rm -f $(BINARY)

install: build
	install -m 755 $(BINARY) $(INSTALL_DIR)/$(BINARY)
	mkdir -p $(DATA_DIR)
	install -m 644 regieleki.service /etc/systemd/system/regieleki.service
	systemctl daemon-reload
	@echo "Run: systemctl enable --now regieleki"

uninstall:
	systemctl stop regieleki || true
	systemctl disable regieleki || true
	rm -f $(INSTALL_DIR)/$(BINARY)
	rm -f /etc/systemd/system/regieleki.service
	systemctl daemon-reload
