package util

import (
	"fmt"
)

type wtoken struct {
	code     byte
	children []*wtoken
	isEnd    bool
}

type wordsFilter struct {
	tokarray [256]*wtoken
}

func insertToken(tok *wtoken, c byte) *wtoken {
	child := &wtoken{
		code:     c,
		isEnd:    false,
		children: make([]*wtoken, 0),
	}

	size := len(tok.children)
	if size == 0 {
		tok.children = append(tok.children, child)
	} else {
		tmp := make([]*wtoken, len(tok.children)+1)
		flag := 0
		for i, v := range tok.children {
			if flag == 0 && v.code > c {
				tmp[i] = child
				flag = 1
			} else {
				tmp[i] = v
			}
		}

		if 0 == flag {
			tmp[size] = child
		} else {
			tmp[size] = tok.children[size-1]
		}

		tok.children = tmp

	}

	return child

}

func getChild(tok *wtoken, c byte) *wtoken {
	size := len(tok.children)
	if 0 == size {
		return nil
	}

	left := 0
	right := size - 1
	for {
		if right-left <= 0 {
			if tok.children[left].code == c {
				return tok.children[left]
			} else {
				return nil
			}
		} else {

			index := (right-left)/2 + left

			if tok.children[index].code == c {
				return tok.children[index]
			} else if tok.children[index].code > c {
				right = index - 1
			} else {
				left = index + 1
			}

		}
	}

	return nil
}

func addChild(tok *wtoken, c byte) *wtoken {
	child := getChild(tok, c)
	if nil == child {
		return insertToken(tok, c)
	} else {
		return child
	}
}

func nextByte(tok *wtoken, str []byte, i int, maxmatch *int) {
	childtok := getChild(tok, str[0])
	if nil != childtok {
		if childtok.isEnd {
			*maxmatch = i + 1
		}

		if len(str) == 1 {
			return
		} else {
			nextByte(childtok, str[1:], i+1, maxmatch)
		}
	} else {
		if tok.isEnd {
			*maxmatch = i
		}
	}
}

func (this *wordsFilter) processWords(str []byte, pos *int) bool {
	tok := this.tokarray[str[*pos]]
	if nil == tok {
		(*pos) += 1
		return true
	} else if len(str) > (*pos + 1) {
		maxmatch := 0
		nextByte(tok, str[(*pos)+1:], (*pos)+1, &maxmatch)
		if 0 == maxmatch {
			*pos += 1
			if tok.isEnd {
				return false
			} else {
				return true
			}
		} else {
			*pos = maxmatch
			return false
		}
	} else {
		return true
	}
}

//检查str中是否包含违禁字符串，如果是返回true
func (this *wordsFilter) Check(str string) bool {

	bytes := []byte(str)

	if len(bytes) == 0 {
		return true
	}

	i := 0
	for i < len(bytes) {
		if !this.processWords(bytes, &i) {
			return false
		}
	}
	return true
}

func NewWrodsFilter(invaildWords []string) *wordsFilter {
	f := &wordsFilter{}
	for _, v := range invaildWords {
		tmp := []byte(v)
		tok := f.tokarray[tmp[0]]
		if nil == tok {
			tok = &wtoken{
				code:     tmp[0],
				isEnd:    false,
				children: make([]*wtoken, 0),
			}
			f.tokarray[tmp[0]] = tok
		}

		for _, vv := range tmp[1:] {
			tok = addChild(tok, vv)
		}
		tok.isEnd = true
	}
	return f
}

func Test() {
	strs := []string{
		"fuck",
		"make love",
		"duck",
		"毛主席",
		"习主席",
		"习主席大大",
	}
	filter := NewWrodsFilter(strs)

	fmt.Println(filter.Check("hello fuck"))

	fmt.Println(filter.Check("make dove"))

	fmt.Println(filter.Check("i like make love"))

	fmt.Println(filter.Check("your ducks"))

	fmt.Println(filter.Check("天安门上毛主席"))

	fmt.Println(filter.Check("天安门上习主席"))
	fmt.Println(filter.Check("天安门上习主席大大"))

	fmt.Println(filter.Check("天安门上老主席"))

}
