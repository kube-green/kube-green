HELM_TMPL_CMD ?= helm template -f charts/kube-green/values.yaml -f charts/kube-green/testValues.yaml
HELM_TMPL_OUT ?= .helm.template-output.yaml
HELM_SNAPSHOT_OUT ?= charts/snapshots/test-output.snap.yaml
tmpl_debug ?=
SHELL=/usr/bin/env bash

##@ Helm Chart utilities
.PHONY: chart-snapshot
chart-snapshot: ## Create a snapshot of the current chart
	@echo "==> Updating snapshot template..."
	$(HELM_TMPL_CMD) $(tmpl_debug) --name-template="release-test" -n default ./charts/kube-green > $(HELM_SNAPSHOT_OUT)
	@sed -i.bak "s|helm\.sh\/chart\:.*|HELM_CHART_VERSION_REDACTED|" $(HELM_SNAPSHOT_OUT)
	@rm $(HELM_SNAPSHOT_OUT).bak

.PHONY: chart-test
chart-test: ## Test the chart against the snapshot
	@echo "==> Running tests..."
	@echo "==> Generating Template from test values..."
	$(HELM_TMPL_CMD) $(tmpl_debug) --name-template="release-test" -n default ./charts/kube-green > $(HELM_TMPL_OUT)
	@sed -i.bak "s|helm\.sh\/chart\:.*|HELM_CHART_VERSION_REDACTED|" $(HELM_TMPL_OUT)
	@rm $(HELM_TMPL_OUT).bak
	diff $(HELM_TMPL_OUT) $(HELM_SNAPSHOT_OUT)
	@echo "==> Tests passed!"

HELM_DOCS = $(shell pwd)/bin/helm-docs
.PHONY: helm-docs-ensure
helm-docs-ensure: ##Download helm-docs locally if necessary.
	$(call go-install-tool,$(HELM_DOCS),github.com/norwoodj/helm-docs/cmd/helm-docs,$(HELM_DOCS_VERSION))

.PHONY: helm-docs
helm-docs: helm-docs-ensure ## Run helm-docs.
	$(HELM_DOCS) --chart-search-root $(shell pwd)/charts
