.PHONY: build clean deps deps-darwin check-go check-wails check-wails3 files files-darwin copia-macos help

BINARY = yang-installer

# Sistema no que se está a compilar. O instalador non se pode
# compilar de forma cruzada (Wails precisa a webview nativa de cada
# plataforma), así que o SO anfitrión decide tamén o SO de destino: en macOS
# hai que empaquetar Yang coma un bundle .app e non coma un binario solto,
# e iso cambia o contido de files/ (ver files-darwin).
UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Darwin)
FILES_TARGET = files-darwin
else
FILES_TARGET = files
endif

## build: Compila o instalador (require Go, wails e node/npm)
## Fronte da propia interface do instalador (frontend/) — non confundir con
## "files"/"files-darwin", que empaquetan o Yang que se instala. Mesmo esquema
## ca installer/Makefile.
build: check-go check-wails assets/logo.png build/appicon.png $(FILES_TARGET)
	go mod tidy
	wails build
ifeq ($(UNAME_S),Darwin)
	@$(MAKE) --no-print-directory copia-macos
endif
	@echo ""
ifeq ($(UNAME_S),Darwin)
	@echo "✅ Compilado: ./yang_installer/build/bin/$(BINARY).app"
	@echo "   Ábreo cun dobre clic, ou:  open ./yang_installer/build/bin/$(BINARY).app"
	@echo "   NON o executes con sudo: en macOS instálase en /Applications sen"
	@echo "   elevar, e Homebrew négase a funcionar coma root."
else
	@echo "✅ Compilado: ./yang_installer/build/bin/$(BINARY)"
	@echo "   Executa con:  sudo ./yang_installer/build/bin/$(BINARY)"
endif

## copia-macos: Refresca yang-installer-macos.app dende o recén compilado.
## Ese nome é o que se distribúe (di explicitamente para que SO é), pero
## "wails build" só produce yang-installer.app: ata agora a copia facíase á
## man e quedaba conxelada. Iso xa causou un fallo real — instalouse un Yang
## de dous días antes, SEN as plantillas de fábrica de plantillas_fabrica.go,
## e a biblioteca de plantillas saía baleira. Ao ir dentro de "build" xa non
## pode desfasarse.
##
## "cp -R" (non "cp"): Contents/_CodeSignature ten que viaxar tamén, senón a
## copia queda sen firmar e Gatekeeper bloquéaa (mesmo motivo ca en
## files-darwin). Verificado: a firma adhoc sobrevive ao cp -R.
copia-macos:
	@rm -rf build/bin/$(BINARY)-macos.app
	@cp -R build/bin/$(BINARY).app build/bin/$(BINARY)-macos.app
	@echo "✅ Copia de distribución ao día: build/bin/$(BINARY)-macos.app"

## assets/logo.png: Copia o logo dende Yang
## Depende de ../yang/logo.png (non abondaba con "assets/logo.png:"
## sen prerequisito: unha vez existise o ficheiro, Make xa non volvía copialo
## aínda que ../yang/logo.png cambiase despois).
assets/logo.png: ../yang/logo.png
	@mkdir -p assets
	@cp ../yang/logo.png assets/logo.png

## build/appicon.png: icona da xanela/taskbar de Wails e logo do frontend
## (mesma orixe ca assets/logo.png, ver nota de enriba).
build/appicon.png: ../yang/logo.png
	@mkdir -p build frontend/src/assets/images
	@cp ../yang/logo.png build/appicon.png
	@cp ../yang/logo.png frontend/src/assets/images/logo.png

## files: Compila Yang e xunta en files/ só o necesario en tempo de execución.
## Todo o demais (help/, codeini_default.ini, icona…) xa vai embebido no
## propio binario "yang" (ver yang/main.go e yang/app.go,
## go:embed). Fonte: ../yang (versión con editor Blockly; ../yang
## quedou como versión anterior, xa non se empaqueta aquí).
## Yang Blockly volveu a Wails v3 (ver historial deste ficheiro - pasara por
## v2 un tempo) - compílase con "task build" (CLI wails3/Taskfile, ver
## yang/Taskfile.yml), non "wails build" (v2). EXTRA_TAGS=gtk3
## fai falla en Linux: é o tag que evita GTK4 (o predeterminado de Wails v3
## beta, que NON compila en Debian 12) forzando o backend GTK3/WebKit2GTK -
## SEN VERIFICAR NUNCA en Debian 12 real dende que se escribiu isto, proba
## primeiro con "task build EXTRA_TAGS=gtk3" a man se falla aquí.
files: check-go check-wails3
	@rm -rf files
	@mkdir -p files
	@cd ../yang && task build EXTRA_TAGS=gtk3
	@cp ../yang/build/bin/yang files/yang
	@cp ../yang/logo.png files/logo.png
	@cp ../yang/modulo.json files/modulo.json
	@chmod +x files/yang
	@echo "✅ yang compilado e ficheiros copiados a yang_installer/files/"

## files-darwin: Igual ca "files" pero para macOS, onde Yang non é un binario
## solto senón un bundle Yang.app. Empaquétase con "task package" (non "task
## build": esa só produce o executable) e cópiase o CONTIDO do bundle —
## Contents/ enteiro — dentro de files/.
##
## Por que o contido e non o Yang.app completo: destDir en macOS xa é
## /Applications/Yang.app (ver install.go), e extractFiles volca files/
## directamente aí dentro. Se en files/ houbese un "Yang.app/", quedaría un
## /Applications/Yang.app/Yang.app. E facer que destDir fose /Applications
## sería moito peor: a desinstalación fai os.RemoveAll(destDir).
##
## "cp -R" e non "cp": Contents/_CodeSignature ten que viaxar tamén (ver a
## nota do //go:embed all:files en install.go).
##
## EXTRA_TAGS=gtk3 NON se pasa aquí: é un arranxo exclusivo de Linux (evitar
## GTK4); en macOS o backend é WKWebView e ese tag non pinta nada.
files-darwin: check-go check-wails3
	@rm -rf files
	@mkdir -p files
	@cd ../yang && task package
	@cp -R ../yang/build/bin/yang.app/Contents files/Contents
	@chmod +x files/Contents/MacOS/yang
	@echo "✅ Yang.app empaquetado e copiado a yang_installer/files/"

## deps-darwin: Dependencias para COMPILAR este instalador nun Mac.
## Non fai falla nada equivalente a libgtk-3-dev: en macOS a webview
## (WKWebView) forma parte do sistema. Si fan falla as Command Line Tools de
## Xcode, que son as que traen o compilador de C que precisa cgo e o
## "codesign" co que se asinan os bundles.
deps-darwin:
	@xcode-select -p > /dev/null 2>&1 || xcode-select --install
	@which brew > /dev/null 2>&1 || (echo "❌ Instala Homebrew primeiro: https://brew.sh" && exit 1)
	brew install go node
	go install github.com/wailsapp/wails/v2/cmd/wails@latest
	go install github.com/wailsapp/wails/v3/cmd/wails3@latest
	brew install go-task
	@echo ""
	@echo "✅ Dependencias instaladas. Engade $$(go env GOPATH)/bin ao PATH se fai falla."
	@echo "   Agora executa: make build"

## deps: Instala as dependencias de sistema necesarias para COMPILAR este
## instalador (non confundir coas dependencias que o instalador INSTALA no
## equipo do profesor — Maxima/LaTeX, ver install.go). Só Debian/Ubuntu:
## nun Mac usa "make deps-darwin".
deps:
	sudo apt-get update
	sudo apt-get install -y \
		golang-go \
		nodejs \
		npm \
		gcc \
		pkg-config \
		libgtk-3-dev \
		libwebkit2gtk-4.0-dev
	go install github.com/wailsapp/wails/v2/cmd/wails@latest
	go install github.com/wailsapp/wails/v3/cmd/wails3@latest
	sudo snap install task --classic || sudo apt-get install -y task
	@echo ""
	@echo "✅ Dependencias instaladas. Agora executa: make build"

## clean: Elimina o binario compilado e ficheiros temporais
clean:
	rm -rf build/bin
	rm -f assets/logo.png build/appicon.png frontend/src/assets/images/logo.png
	rm -rf frontend/dist frontend/wailsjs
	rm -rf files

check-go:
	@which go > /dev/null 2>&1 || (echo "❌ Go non está instalado. Executa: make deps" && exit 1)

check-wails:
	@which wails > /dev/null 2>&1 || (echo "❌ wails non está instalado. Executa: go install github.com/wailsapp/wails/v2/cmd/wails@latest" && exit 1)

## check-wails3: para "files" (compila yang, que está en Wails v3).
check-wails3:
	@which wails3 > /dev/null 2>&1 || (echo "❌ wails3 non está instalado. Executa: go install github.com/wailsapp/wails/v3/cmd/wails3@latest" && exit 1)
	@which task > /dev/null 2>&1 || (echo "❌ task non está instalado (require wails3 build). Ver https://taskfile.dev/installation/" && exit 1)

help:
	@echo ""
	@echo "Instalador de Yang — Comandos dispoñibles:"
	@echo ""
	@grep -E '^## ' Makefile | sed 's/## /  /'
	@echo ""
