BUILD = upstream
BUILD_DIR = ../docs_build
ROOTDIR = $(realpath .)
NAME = watcher-operator
DEST_DIR = $(BUILD_DIR)/$(NAME)
DEST_HTML = $(DEST_DIR)/index.html
INSTALL_HTML = $(DEST_DIR)/install-$(BUILD).html
WORKFLOW_HTML = $(DEST_DIR)/workflow-$(BUILD).html
IMAGES_DIR = $(DEST_DIR)/images
IMAGES_TS = $(DEST_DIR)/.timestamp-images
MAIN_SOURCE = main.adoc
INSTALL_SOURCE = install_guide.adoc
WORKFLOW_SOURCE = user_workflow.adoc
OTHER_SOURCES = $(shell find ./assemblies -type f)
IMAGES = $(shell find ./images -type f)
ALL_SOURCES = $(MAIN_SOURCE) $(OTHER_SOURCES) $(IMAGES)
UNAME = $(shell uname)
BUNDLE_EXEC ?= bundle exec

ifeq ($(UNAME), Linux)
BROWSER_OPEN = xdg-open
endif
ifeq ($(UNAME), Darwin)
BROWSER_OPEN = open
endif

all: html

html: html-latest

html-latest: prepare $(IMAGES_TS) $(DEST_HTML) $(INSTALL_HTML) $(WORKFLOW_HTML)

prepare:
	@mkdir -p $(BUILD_DIR)
	@mkdir -p $(DEST_DIR) $(IMAGES_DIR)

clean:
	@rm -rf "$(DEST_DIR)"

watch-html:
	@which inotifywait > /dev/null || ( echo "ERROR: inotifywait not found, install inotify-tools" && exit 1 )
	while true; do \
		inotifywait -r -e modify -e create -e delete .; \
		sleep 0.5; \
		$(MAKE) html; \
	done

open-html: html
	${BROWSER_OPEN} "file://$(realpath $(ROOTDIR)/$(DEST_HTML))"

$(IMAGES_TS): $(IMAGES)
	cp $? $(IMAGES_DIR)
	touch $(IMAGES_TS)

$(DEST_HTML): $(ALL_SOURCES)
	$(BUNDLE_EXEC) asciidoctor -a source-highlighter=highlightjs -a highlightjs-languages="yaml,bash" -a highlightjs-theme="monokai" --failure-level WARN -a build=$(BUILD) -b xhtml5 -d book -o $@ $(MAIN_SOURCE)

$(INSTALL_HTML): $(ALL_SOURCES)
	$(BUNDLE_EXEC) asciidoctor -a source-highlighter=highlightjs -a highlightjs-languages="yaml,bash" -a highlightjs-theme="monokai" --failure-level WARN -a build=$(BUILD) -b xhtml5 -d book -o $@ $(INSTALL_SOURCE)

$(WORKFLOW_HTML): $(ALL_SOURCES)
	$(BUNDLE_EXEC) asciidoctor -a source-highlighter=highlightjs -a highlightjs-languages="yaml,bash" -a highlightjs-theme="monokai" --failure-level WARN -a build=$(BUILD) -b xhtml5 -d book -o $@ $(WORKFLOW_SOURCE)
