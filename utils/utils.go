package utils

import "net"

func IPsMatch(l, r []net.IP) bool {
	if len(l) != len(r) {
		return false
	}

	ips := make(map[string]struct{}, len(l))
	for _, ip := range l {
		ips[ip.String()] = struct{}{}
	}

	for _, ip := range r {
		if _, matches := ips[ip.String()]; !matches {
			return false
		}
	}

	return true
}

func IPsOverlap(l, r []net.IP) bool {
	if len(l) == 0 || len(r) == 0 {
		return false
	}

	ips := make(map[string]struct{}, len(l))
	for _, ip := range l {
		ips[ip.String()] = struct{}{}
	}

	for _, ip := range r {
		if _, matches := ips[ip.String()]; !matches {
			return true
		}
	}

	return false
}

func IsLoopback(ips []net.IP) bool {
	for _, ip := range ips {
		if !ip.IsLoopback() {
			return false
		}
	}

	return true
}

func LocalIPs() ([]net.IP, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	res := make([]net.IP, 0)

	for _, ifs := range interfaces {
		if ifs.Flags&net.FlagUp == 0 {
			continue
		}

		addrs, err := ifs.Addrs()
		if err != nil {
			return nil, err
		}

		for _, addr := range addrs {
			switch ip := addr.(type) {
			case *net.IPNet:
				if !ip.IP.IsLoopback() {
					res = append(res, ip.IP)
				}
			case *net.IPAddr:
				if !ip.IP.IsLoopback() {
					res = append(res, ip.IP)
				}
			case *net.TCPAddr:
				if !ip.IP.IsLoopback() {
					res = append(res, ip.IP)
				}
			case *net.UDPAddr:
				if !ip.IP.IsLoopback() {
					res = append(res, ip.IP)
				}
			}
		}
	}

	return res, nil
}
