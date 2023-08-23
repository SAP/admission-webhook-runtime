ENVTEST_K8S_VERSION = 1.26.1

BASEPATH := $(abspath $(lastword $(MAKEFILE_LIST)))
BASEDIR := $(dir $(BASEPATH))

.PHONY: test
test: envtest
	@KUBEBUILDER_ASSETS=$(BASEDIR)/envtest/current go test ./pkg/...

.PHONY: envtest
envtest: setupenvtest
	@$(SETUPENVTEST) use --bin-dir $(BASEDIR)/envtest $(ENVTEST_K8S_VERSION)
	@ENVTESTDIR=$$($(SETUPENVTEST) use --bin-dir $(BASEDIR)/envtest $(ENVTEST_K8S_VERSION) -p path) ;\
	chmod -R u+w $$ENVTESTDIR ;\
	rm -f $(BASEDIR)/envtest/current ;\
	ln -s $$ENVTESTDIR $(BASEDIR)/envtest/current

SETUPENVTEST = $(BASEDIR)/bin/setup-envtest
.PHONY: setupenvtest
setupenvtest:
	$(call go-install-tool,$(SETUPENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest@latest)

define go-install-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(BASEDIR)/bin go install $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef
