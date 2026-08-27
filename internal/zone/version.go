package zone

import "zonedns/internal/model"

const maxSerial = uint32(1<<32 - 1)

func NextSerial(current uint32) uint32 {
	if current >= maxSerial {
		return 1
	}
	return current + 1
}

func RangeBetween(from, to uint32) model.SerialRange {
	return model.SerialRange{Start: from, End: to}
}
