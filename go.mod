module code.cloudfoundry.org/tps

go 1.16

require (
	code.cloudfoundry.org/bbs v0.0.0-20220325145300-b2855629fde1
	code.cloudfoundry.org/clock v1.0.0
	code.cloudfoundry.org/consuladapter v0.0.0-20211122211027-9dbbfa656ee0 // indirect
	code.cloudfoundry.org/debugserver v0.0.0-20200131002057-141d5fa0e064
	code.cloudfoundry.org/diego-logging-client v0.0.0-20220314190632-277a9c460661 // indirect
	code.cloudfoundry.org/durationjson v0.0.0-20200131001738-04c274cd71ed // indirect
	code.cloudfoundry.org/executor v0.0.0-20201214152003-d98dd1d962d6 // indirect
	code.cloudfoundry.org/go-diodes v0.0.0-20220325013804-800fb6f70e2f // indirect
	code.cloudfoundry.org/lager v2.0.0+incompatible
	code.cloudfoundry.org/localip v0.0.0-20200131001204-30f63a0935f5
	code.cloudfoundry.org/locket v0.0.0-20220325152040-ad30c800960d
	code.cloudfoundry.org/rep v0.0.0-20210223164058-636ff033bfc3 // indirect
	code.cloudfoundry.org/runtimeschema v0.0.0-20210817192503-36a2cb16a206
	code.cloudfoundry.org/tlsconfig v0.0.0-20211123175040-23cc9f05b6b3 // indirect
	code.cloudfoundry.org/urljoiner v0.0.0-20170223060717-5cabba6c0a50
	code.cloudfoundry.org/workpool v0.0.0-20200131000409-2ac56b354115
	github.com/armon/go-metrics v0.3.10 // indirect
	github.com/bmizerany/pat v0.0.0-20210406213842-e4b6760bdd6f // indirect
	github.com/cloudfoundry/dropsonde v1.0.0
	github.com/cloudfoundry/sonde-go v0.0.0-20220324234026-9851b3a0dce2 // indirect
	github.com/fatih/color v1.13.0 // indirect
	github.com/go-sql-driver/mysql v1.5.0 // indirect
	github.com/go-test/deep v1.0.7 // indirect
	github.com/hashicorp/consul/api v1.12.0 // indirect
	github.com/hashicorp/go-hclog v1.2.0 // indirect
	github.com/hashicorp/go-immutable-radix v1.3.1 // indirect
	github.com/hashicorp/serf v0.9.7 // indirect
	github.com/jackc/pgx v3.6.2+incompatible // indirect
	github.com/lib/pq v1.9.0
	github.com/mattn/go-colorable v0.1.12 // indirect
	github.com/mitchellh/mapstructure v1.4.3 // indirect
	github.com/onsi/ginkgo v1.16.5
	github.com/onsi/gomega v1.18.1
	github.com/tedsuo/ifrit v0.0.0-20191009134036-9a97d0632f00
	github.com/tedsuo/rata v1.0.0 // indirect
	github.com/vito/go-sse v1.0.0 // indirect
	golang.org/x/net v0.0.0-20220325170049-de3da57026de // indirect
	golang.org/x/sys v0.0.0-20220328115105-d36c6a25d886 // indirect
	google.golang.org/genproto v0.0.0-20220328180837-c47567c462d1 // indirect
)

replace github.com/hashicorp/consul => github.com/hashicorp/consul v1.8.1

replace github.com/irconus-labs/circonusllhist => github.com/openhistorgram/circonusllhist v0.3.0
