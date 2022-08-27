# cf4

One file which contains all possible bytes (0..255) and also several different line ending combinations in order to check for possible pre-processing issues with digest creation. See <https://github.com/OCFL/spec/issues/39>.

```
simeon@RottenApple ~> perl -e 'open(my $fh, "> :raw :bytes", "a"); foreach $j (0...255) { print {$fh} "$j ".chr($j)."\n"; } print {$fh} "line endings: \r\n \n\r \r \n"; close($fh)'
simeon@RottenApple ~> shasum -t a; shasum -b a; shasum -p a
f7867717259f8026e014e4c56e1b4683c049e80c  a
f7867717259f8026e014e4c56e1b4683c049e80c *a
f7867717259f8026e014e4c56e1b4683c049e80c ?a
```

