{
	#Inventory

	#Inventory: {
		id:                string
		type:              *"https://ocfl.io/1.0/spec/#inventory" | string
		digestAlgorithm:   #Algs
		head:              =~"v[0-9]+"
		contentDirectory?: *"content" | string
		manifest:          #Manifest
		versions: [=~"v[0-9]+"]: #Version
		fixity: [#Algs]: [string]: [...string]
	}

	#Manifest: [=~"[a-z0-9]+"]: [... string]
	#Manifest: [string]: [... !~"^[.\/]"]
	#Manifest: [string]: [... !~"/$"]

	#Version: {
		created: =~"^([0-9]{4})-([0-9]{2})-([0-9]{2})([Tt]([0-9]{2}):([0-9]{2}):([0-9]{2})(\\.[0-9]+)?)?(([Zz]|([+-])([0-9]{2}):([0-9]{2})))"
		state: [string]: [string, ...]
		message?: string
		user?: {
			name:     string
			address?: string
		}
	}

	#Version: state: [=~"[a-z0-9]+"]: [... string]
	#Version: state: [string]: [... !~"^[.\/]"]
	#Version: state: [string]: [... !~"/$"]

	#Algs: "md5" | "sha256" | "sha512" | "sha1" | "blake2b-512"
}
