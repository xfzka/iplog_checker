package main

import (
	"net/netip"
	"sync"
)

// CIDRNode 字典树节点
type CIDRNode struct {
	children [2]*CIDRNode // 0: left, 1: right
	end      bool
}

// NetList 存储单 IP 和 CIDR 的混合列表
type NetList struct {
	ips      map[uint32]struct{} // HASH Set 存储精确 IP
	cidrRoot *CIDRNode
}

// NewNetList 创建新的 NetList
func NewNetList(ips []uint32, cidrs []netip.Prefix) *NetList {
	nl := &NetList{
		ips:      make(map[uint32]struct{}),
		cidrRoot: &CIDRNode{},
	}

	for _, ip32 := range ips {
		nl.ips[ip32] = struct{}{}
	}

	for _, prefix := range cidrs {
		// ipToUint32 将 netip.Addr 转换为 uint32 (大端序逻辑适配 Trie)
		if prefix.IsValid() && prefix.Addr().Is4() {
			addr := prefix.Addr()
			bits := prefix.Bits()
			node := nl.cidrRoot
			ip32 := ipToUint32(addr)
			for i := 31; i >= 32-bits; i-- {
				bit := (ip32 >> i) & 1
				if node.children[bit] == nil {
					node.children[bit] = &CIDRNode{}
				}
				node = node.children[bit]
			}
			node.end = true
		}
	}

	return nl
}

// Contains 检查 IP 是否在列表中
func (nl *NetList) Contains(ip uint32) bool {
	// 先检查精确 IP
	if _, ok := nl.ips[ip]; ok {
		return true
	}

	// tire 查找 cidr
	node := nl.cidrRoot
	matched := false
	for i := 31; i >= 0; i-- {
		if node == nil {
			break
		}
		if node.end {
			matched = true
		}
		bit := (ip >> i) & 1
		node = node.children[bit]
	}
	if node != nil && node.end {
		matched = true
	}
	return matched
}

// ListInfo 数据源信息
type ListInfo struct {
	Name  string // 数据源名称
	Level int    // 数据源等级
}

// NewNetListInfo 创建新的 NetListInfo
func NewNetListInfo(name string, level int) ListInfo {
	return ListInfo{
		Name:  name,
		Level: level,
	}
}

// ListGroup 管理多个 NetList
type ListGroup struct {
	AllList map[ListInfo]*NetList
}

// NewListGroup 创建新的 ListGroup
func NewListGroup() *ListGroup {
	return &ListGroup{
		AllList: make(map[ListInfo]*NetList),
	}
}

// AddList 添加新的 NetList 到 ListGroup
func (lg *ListGroup) AddList(info ListInfo, ips []uint32, cidrs []netip.Prefix) {
	nl := NewNetList(ips, cidrs)
	lg.AllList[info] = nl
}

// DelList 从 ListGroup 中删除指定名称的 NetList
func (lg *ListGroup) DelList(name string) {
	for k := range lg.AllList {
		if k.Name == name {
			delete(lg.AllList, k)
			return
		}
	}
}

// Contains 检查 IP 是否在任何 NetList 中, 并发检查以提高效率, 任意一个匹配则返回 true 和对应的 NetListInfo
func (lg *ListGroup) Contains(ip uint32) (bool, ListInfo) {
	var wg sync.WaitGroup
	found := make(chan struct {
		bool
		ListInfo
	}, 1)

	// 并发检查每个 NetList 是否包含指定 IP
	for info, nl := range lg.AllList {
		wg.Add(1)
		go func(i ListInfo, n *NetList) {
			defer wg.Done()
			if n.Contains(ip) {
				select {
				case found <- struct { // 找到任意一个就立即返回
					bool
					ListInfo
				}{true, i}:
				default:
				}
			}
		}(info, nl)
	}

	// 等待所有 goroutine 完成后发送结果
	go func() {
		wg.Wait()
		select {
		case found <- struct {
			bool
			ListInfo
		}{false, ListInfo{}}:
		default:
		}
	}()

	res := <-found
	return res.bool, res.ListInfo
}

// IsIPInSafeList 检查 IP 是否在安全列表中
func IsIPInSafeList(ip uint32) bool {
	if SafeListData == nil {
		return false
	}
	found, _ := SafeListData.Contains(ip)
	return found
}

// IsSensitiveIP 检测IP是否敏感
func IsSensitiveIP(ip uint32) (bool, ListInfo) {
	// 判断是否在安全列表中（白名单）
	if IsIPInSafeList(ip) {
		return false, ListInfo{}
	}
	// 判断是否在风险IP列表中
	if RiskListData == nil {
		return false, ListInfo{}
	}
	found, info := RiskListData.Contains(ip)
	if found {
		return true, info
	}
	return false, ListInfo{}
}
