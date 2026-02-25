package main

import (
	"net/netip"
	"sync"
)

// countCIDRNodes 递归统计 CIDR trie 树中的节点数（即 CIDR 条目数）
func countCIDRNodes(node *CIDRNode) int {
	if node == nil {
		return 0
	}
	count := 0
	if node.end {
		count = 1
	}
	count += countCIDRNodes(node.children[0])
	count += countCIDRNodes(node.children[1])
	return count
}

// NetList 存储单 IP 和 CIDR 的混合列表
type NetList struct {
	ips      map[uint32]struct{} // HASH Set 存储精确 IP
	cidrRoot *CIDRNode
}

// CIDRNode 字典树节点
type CIDRNode struct {
	children [2]*CIDRNode // 0: left, 1: right
	end      bool
}

// ListInfo 数据源信息
type ListInfo struct {
	Name  string // 数据源名称
	Level int    // 数据源等级
}

// ListGroup 管理多个 NetList
type ListGroup struct {
	mu      sync.RWMutex
	AllList map[ListInfo]*NetList
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

// NewNetListInfo 创建新的 NetListInfo
func NewNetListInfo(name string, level int) ListInfo {
	return ListInfo{
		Name:  name,
		Level: level,
	}
}

// NewListGroup 创建新的 ListGroup
func NewListGroup() *ListGroup {
	return &ListGroup{
		AllList: make(map[ListInfo]*NetList),
	}
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

// AddList 添加新的 NetList 到 ListGroup (线程安全)
func (lg *ListGroup) AddList(info ListInfo, ips []uint32, cidrs []netip.Prefix) {
	nl := NewNetList(ips, cidrs)
	lg.mu.Lock()
	lg.AllList[info] = nl
	lg.mu.Unlock()
}

// DelList 从 ListGroup 中删除指定名称的 NetList (线程安全)
func (lg *ListGroup) DelList(name string) {
	lg.mu.Lock()
	defer lg.mu.Unlock()
	for k := range lg.AllList {
		if k.Name == name {
			delete(lg.AllList, k)
			return
		}
	}
}

// Contains 检查 IP 是否在任何 NetList 中 (线程安全)
// 顺序检查所有 NetList，map+trie 查找本身极快，无需 goroutine 开销
func (lg *ListGroup) Contains(ip uint32) (bool, ListInfo) {
	lg.mu.RLock()
	defer lg.mu.RUnlock()

	for info, nl := range lg.AllList {
		if nl.Contains(ip) {
			return true, info
		}
	}
	return false, ListInfo{}
}

// Stats 返回统计信息：总条目数和每个列表的条目数 (线程安全)
func (lg *ListGroup) Stats() (totalCount int, perList map[string]int) {
	lg.mu.RLock()
	defer lg.mu.RUnlock()

	perList = make(map[string]int)
	for info, nl := range lg.AllList {
		ipCount := len(nl.ips)
		cidrCount := countCIDRNodes(nl.cidrRoot)
		total := ipCount + cidrCount
		totalCount += total
		perList[info.Name] = total
	}
	return
}
