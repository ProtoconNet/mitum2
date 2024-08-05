package launch

import "github.com/ProtoconNet/mitum2/util/ps"

func DefaultImportPS() *ps.PS {
	pps := ps.NewPS("cmd-import")

	_ = pps.
		AddOK(PNameEncoder, PEncoder, nil).
		AddOK(PNameDesign, PLoadDesign, nil, PNameEncoder).
		AddOK(PNameTimeSyncer, PStartTimeSyncer, PCloseTimeSyncer, PNameDesign).
		AddOK(PNameLocal, PLocal, nil, PNameDesign).
		AddOK(PNameBlockItemReaders, PBlockItemReaders, nil, PNameDesign).
		AddOK(PNameStorage, PStorage, PCloseStorage, PNameLocal)

	_ = pps.POK(PNameEncoder).
		PostAddOK(PNameAddHinters, PAddHinters)

	_ = pps.POK(PNameDesign).
		PostAddOK(PNameCheckDesign, PCheckDesign).
		PostAddOK(PNameINITObjectCache, PINITObjectCache)

	_ = pps.POK(PNameBlockItemReaders).
		PreAddOK(PNameBlockItemReadersDecompressFunc, PBlockItemReadersDecompressFunc).
		PostAddOK(PNameRemotesBlockItemReaderFunc, PRemotesBlockItemReaderFunc)

	_ = pps.POK(PNameStorage).
		PreAddOK(PNameCheckLocalFS, PCheckAndCreateLocalFS).
		PreAddOK(PNameLoadDatabase, PLoadDatabase).
		PostAddOK(PNameCheckLeveldbStorage, PCheckLeveldbStorage).
		PostAddOK(PNameLoadFromDatabase, PLoadFromDatabase).
		PostAddOK(PNameCheckBlocksOfStorage, PCheckBlocksOfStorage).
		PostAddOK(PNamePatchBlockItemReaders, PPatchBlockItemReaders)

	return pps
}
