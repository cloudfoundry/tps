module code.cloudfoundry.org/tps

go 1.20

require (
	code.cloudfoundry.org/bbs v0.0.0-20230406145249-41bd09f9f0ca
	code.cloudfoundry.org/clock v1.1.0
	code.cloudfoundry.org/debugserver v0.0.0-20230329140605-8c21649a9a42
	code.cloudfoundry.org/lager/v3 v3.0.1
	code.cloudfoundry.org/localip v0.0.0-20230406154046-f137f65d303d
	code.cloudfoundry.org/locket v0.0.0-20230406154009-5e8522d975d2
	code.cloudfoundry.org/runtimeschema v0.0.0-20230323223330-5366865eed76
	code.cloudfoundry.org/urljoiner v0.0.0-20170223060717-5cabba6c0a50
	code.cloudfoundry.org/workpool v0.0.0-20230406174608-2e26d5d93731
	github.com/cloudfoundry/dropsonde v1.0.0
	github.com/lib/pq v1.10.7
	github.com/onsi/ginkgo/v2 v2.9.2
	github.com/onsi/gomega v1.27.6
	github.com/tedsuo/ifrit v0.0.0-20230330192023-5cba443a66c4
)

require (
	code.cloudfoundry.org/cfhttp/v2 v2.0.1-0.20210513172332-4c5ee488a657 // indirect
	code.cloudfoundry.org/diego-logging-client v0.0.0-20220314190632-277a9c460661 // indirect
	code.cloudfoundry.org/durationjson v0.0.0-20200131001738-04c274cd71ed // indirect
	code.cloudfoundry.org/go-diodes v0.0.0-20220325013804-800fb6f70e2f // indirect
	code.cloudfoundry.org/go-loggregator/v8 v8.0.5 // indirect
	code.cloudfoundry.org/tlsconfig v0.0.0-20230320190829-8f91c367795b // indirect
	filippo.io/edwards25519 v1.0.0-rc.1 // indirect
	github.com/bmizerany/pat v0.0.0-20210406213842-e4b6760bdd6f // indirect
	github.com/cloudfoundry/gosteno v0.0.0-20150423193413-0c8581caea35 // indirect
	github.com/cloudfoundry/sonde-go v0.0.0-20220324234026-9851b3a0dce2 // indirect
	github.com/go-logr/logr v1.2.4 // indirect
	github.com/go-task/slim-sprig v0.0.0-20230315185526-52ccab3ef572 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/google/pprof v0.0.0-20230323073829-e72429f035bd // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/nu7hatch/gouuid v0.0.0-20131221200532-179d4d0c4d8d // indirect
	github.com/openzipkin/zipkin-go v0.4.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/square/certstrap v1.3.0 // indirect
	github.com/tedsuo/rata v1.0.0 // indirect
	github.com/vito/go-sse v1.0.0 // indirect
	go.step.sm/crypto v0.16.2 // indirect
	golang.org/x/crypto v0.1.0 // indirect
	golang.org/x/net v0.8.0 // indirect
	golang.org/x/sys v0.6.0 // indirect
	golang.org/x/text v0.8.0 // indirect
	golang.org/x/tools v0.7.0 // indirect
	google.golang.org/genproto v0.0.0-20230110181048-76db0878b65f // indirect
	google.golang.org/grpc v1.54.0 // indirect
	google.golang.org/protobuf v1.28.1 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/hashicorp/consul => github.com/hashicorp/consul v1.8.1

replace github.com/irconus-labs/circonusllhist => github.com/openhistorgram/circonusllhist v0.3.0
