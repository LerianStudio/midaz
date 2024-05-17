package mpostgres

// RegexIgnoreAccents receives a regex, than, for each char it's adds the accents variations to expression
// Ex: Given "a" -> "aáàãâ"
// Ex: Given "c" -> "ç"
func RegexIgnoreAccents(regex string) string {
	m1 := map[string]string{
		"a": "[aáàãâ]",
		"e": "[eéèê]",
		"i": "[iíìî]",
		"o": "[oóòõô]",
		"u": "[uùúû]",
		"c": "[cç]",
		"A": "[AÁÀÃÂ]",
		"E": "[EÉÈÊ]",
		"I": "[IÍÌÎ]",
		"O": "[OÓÒÕÔ]",
		"U": "[UÙÚÛ]",
		"C": "[CÇ]",
	}
	m2 := map[string]string{
		"a": "a",
		"á": "a",
		"à": "a",
		"ã": "a",
		"â": "a",
		"e": "e",
		"é": "e",
		"è": "e",
		"ê": "e",
		"i": "i",
		"í": "i",
		"ì": "i",
		"î": "i",
		"o": "o",
		"ó": "o",
		"ò": "o",
		"õ": "o",
		"ô": "o",
		"u": "u",
		"ù": "u",
		"ú": "u",
		"û": "u",
		"c": "c",
		"ç": "c",
		"A": "A",
		"Á": "A",
		"À": "A",
		"Ã": "A",
		"Â": "A",
		"E": "E",
		"É": "E",
		"È": "E",
		"Ê": "E",
		"I": "I",
		"Í": "I",
		"Ì": "I",
		"Î": "I",
		"O": "O",
		"Ó": "O",
		"Ò": "O",
		"Õ": "O",
		"Ô": "O",
		"U": "U",
		"Ù": "U",
		"Ú": "U",
		"Û": "U",
		"C": "C",
		"Ç": "C",
	}
	s := ""

	for _, ch := range regex {
		c := string(ch)
		if v1, found := m2[c]; found {
			if v2, found2 := m1[v1]; found2 {
				s += v2
				continue
			}
		}

		s += string(ch)
	}

	return s
}

// RemoveChars from a string
func RemoveChars(str string, chars map[string]bool) string {
	s := ""

	for _, ch := range str {
		c := string(ch)
		if _, found := chars[c]; found {
			continue
		}

		s += string(ch)
	}

	return s
}
