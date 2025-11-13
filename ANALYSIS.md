# Performance analysis for Perseus Citation Processor

As of Nov. 13 2025.

## Data

All the XML files currently contained in a private version
of the Perseus [repo for commentaries](https://github.com/PerseusDL/canonical-pdlrefwk).
This amounts to 125 files with a total of 2070827 lines of XML.

## Resolved and unresolved citations

Running the citation processor on all of these files generates
`resolved.jsonl` with 216098 lines and `unresolved.jsonl`
with 30470 lines, for a total of 246568 identified citations.
This amounts to a resolution rate of 0.876, which is quite good
given that a large number number of the unresolved citations
appear to be to modern scholarly works. Although the processor
is set up to catch some common reference works, the goal here
is primarily to deal with citations to ancient works, so
it's not a big deal to fail to resolve many of these citations.

It's worth noting, however, that just because the processor
resolves a citation doesn't mean that it resolves it correctly.
In particular, any ambiguous citations that get resolved incorrectly
will still wind up in `resolved.jsonl`. In addition,
any citations where the processor recognizes the author but
not the work will also wind up in `resolved.jsonl`.

Running the processor here takes 0m28.374s,
with 4m28.474s spent in user space. This is across
32 workers using 32 threads. The time here
isn't especially important, but does confirm
that parallel processing makes smoothing out
wrinkles in the citation processing quite a bit
less annoying.
