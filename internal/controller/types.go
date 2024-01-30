package controller

type DockerRegistrySetStatus string

const (

	//初始化
	INITIAL DockerRegistrySetStatus = "Initial"

	//secrets更新
	UPDATE DockerRegistrySetStatus = "Update"

	//namespace新增
	NSADD DockerRegistrySetStatus = "NsAdd"

	//secrets已经全部刷新
	ALLOCATED DockerRegistrySetStatus = "Allocated"

	////两种中间状态
	//NSADDPORTION = "NsAdding"
	//
	////两种中间状态
	//NSDELPORTION = "NsDeling"

	// 两种失败状态

	Failed DockerRegistrySetStatus = "Failed"
)

type Counter struct {
	times int
	total int
}

func NewCounter(total int) *Counter {
	return &Counter{
		total: total,
	}
}

func (c *Counter) Add() {
	c.times++
}

func (c *Counter) Count() int {
	return c.times
}

func (c *Counter) Result() bool {
	return c.total == c.times
}
