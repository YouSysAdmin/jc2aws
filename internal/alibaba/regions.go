package alibaba

// RegionsList enumerates the public Alibaba Cloud region IDs offered in the
// interactive picker.
//
// The list is a convenience, not a constraint: ValidateRegion accepts any
// syntactically valid region ID, because Alibaba Cloud adds regions faster than
// this tool cuts releases, and partition regions (finance, gov) are omitted
// here on purpose.
//
// Note that several IDs collide with AWS region IDs while naming entirely
// different places: eu-central-1 is Frankfurt, us-west-1 is Silicon Valley,
// eu-west-1 is London and na-south-1 is Mexico. Never reconcile this list
// against the AWS one.
var RegionsList = []string{
	// China
	"cn-qingdao", "cn-beijing", "cn-zhangjiakou", "cn-huhehaote", "cn-wulanchabu",
	"cn-hangzhou", "cn-shanghai", "cn-nanjing", "cn-fuzhou",
	"cn-shenzhen", "cn-heyuan", "cn-guangzhou", "cn-chengdu", "cn-zhongwei",
	"cn-hongkong",
	// Asia Pacific
	"ap-northeast-1", "ap-northeast-2",
	"ap-southeast-1", "ap-southeast-3", "ap-southeast-5", "ap-southeast-6", "ap-southeast-7",
	// Europe and the Americas
	"us-west-1", "us-east-1",
	"eu-central-1", "eu-west-1",
	"na-south-1",
	// Middle East
	"me-east-1", "me-central-1",
}
