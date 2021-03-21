{
	#Inventory
	#Inventory: {
		id:                Id
		type:              *"https://ocfl.io/1.0/spec/#inventory" | string
		digestAlgorithm:   Algs
		head:              Version
		contentDirectory?: *"content" | string
		manifest:          #ContentMap
		versions: [Version]: #Version
		versions: [head]: #Version
		fixity: [Algs]: #ContentMap
	}

	#Version: {
		created:  =~"^([0-9]{4})-([0-9]{2})-([0-9]{2})([Tt]([0-9]{2}):([0-9]{2}):([0-9]{2})(\\.[0-9]+)?)?(([Zz]|([+-])([0-9]{2}):([0-9]{2})))"
		state:    #ContentMap
		message?: string
		user?: {
			name:     string
			address?: string
		}
	}

	#ContentMap: [Digest]: [... Paths]

	let Algs = "md5" | "sha256" | "sha512" | "sha1" | "blake2b-512"
	let Id = !=""
	let Paths = !~"^[\/]" & !~"/$"
	let Digest = =~"^[a-zA-Z0-9]+$"
	let Version = =~"^v[0-9]+$"
}
