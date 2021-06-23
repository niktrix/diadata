package main

import (
	"fmt"
	filters "github.com/diadata-org/diadata/internal/pkg/filtersOptionService"
	"github.com/diadata-org/diadata/pkg/dia"
	models "github.com/diadata-org/diadata/pkg/model"
	"github.com/sirupsen/logrus"
	"math"
	"sort"
	"strings"
	"time"
)

// Get Near Term options
// Get NExt Term Options

var log = logrus.New()

type UnderlyingAsset struct {
	Symbol       string
	InterestRate float64
}

var underlyingAsset map[string]UnderlyingAsset

func init() {

	underlyingAsset = make(map[string]UnderlyingAsset)
	underlyingAsset["ETH"] = UnderlyingAsset{Symbol: "ETH", InterestRate: 0.01}
	underlyingAsset["BTC"] = UnderlyingAsset{Symbol: "BTC", InterestRate: 0.0054}

}

type OptionsTable struct {
	StrikePrice          float64
	PutAsk               float64
	PutBid               float64
	CallAsk              float64
	CallBid              float64
	CallMid              float64
	PutMid               float64
	Difference           float64
	ContributionByStrike float64
	Type                 dia.OptionType
	DeltaK               float64
}

func main() {
	asset := underlyingAsset["ETH"]

	// Get Data source
	ds, err := models.NewDataStore()
	if err != nil {
		log.Errorln("Error Getting Datastore", err)
	}
	for {
		CalculateVIX(asset, ds)
		time.Sleep(5 * time.Minute)
	}

}

func CalculateVIX(asset UnderlyingAsset, ds *models.DB) {
	var (
		nearTerm         map[float64]OptionsTable // All near Term Options
		nextTerm         map[float64]OptionsTable // All next Term Options
		nearTermCombined map[string]OptionsTable  //   near Term Options combined,call and Put separated as key
		nextTermCombined map[string]OptionsTable  //   next Term Options combined,call and Put separated as key
	)
	nearTerm = make(map[float64]OptionsTable)
	nextTerm = make(map[float64]OptionsTable)
	nearTermCombined = make(map[string]OptionsTable)
	nextTermCombined = make(map[string]OptionsTable)
	options, error := filters.GetOptionComponents(asset.Symbol)

	if error != nil {
		log.Error("Error getting Option Components", error)
		return
	}

	// Calculate T
	t1 := filters.CalculateT(510.0, 34560.0)
	t2 := filters.CalculateT(900.0, 44640.0)

	nextTermOption, _ := filters.GetNext(options)
	nearTermOptions, _ := filters.GetNear(options)

	// Array for strike Prices, helps in sorting
	var nearstrikePrices []float64
	var nextstrikePrices []float64

	log.Infoln("Total Options selected", len(options))
	log.Infoln("Total Next Term Options", len(nextTermOption))
	log.Infoln("Total Near Term Options", len(nearTermOptions))

	// Create map of strike Price and call, bid, difference
	for _, option := range nextTermOption {
		ot := OptionsTable{}
		v, ok := nextTerm[option.StrikePrice]
		if ok {
			ot = v
		}

		isExist := false
		for _, strikePrice := range nextstrikePrices {
			if strikePrice == option.StrikePrice {
				isExist = true
			}
		}
		if !isExist {
			nextstrikePrices = append(nextstrikePrices, option.StrikePrice)
		}

		ot.StrikePrice = option.StrikePrice

		// Get orderbook for option
		orderbook, _ := ds.GetOptionOrderbookDataInflux(option)
		ot.Type = option.OptionType

		if option.OptionType == dia.CallOption {
			ot.CallAsk = orderbook.AskPrice
			ot.CallBid = orderbook.BidPrice
			strike := fmt.Sprintf("%f", option.StrikePrice)
			nextTermCombined[strike+"-C"] = ot

		}
		if option.OptionType == dia.PutOption {
			ot.PutAsk = orderbook.AskPrice
			ot.PutBid = orderbook.BidPrice
			strike := fmt.Sprintf("%f", option.StrikePrice)
			nextTermCombined[strike+"-P"] = ot
		}

		nextTerm[option.StrikePrice] = ot

	}

	for _, option := range nearTermOptions {
		ot := OptionsTable{}
		v, ok := nearTerm[option.StrikePrice]
		if ok {
			ot = v
		}

		isExist := false
		for _, strikePrice := range nearstrikePrices {
			if strikePrice == option.StrikePrice {
				isExist = true
			}
		}
		if !isExist {
			nearstrikePrices = append(nearstrikePrices, option.StrikePrice)
		}

		ot.StrikePrice = option.StrikePrice

		orderbook, _ := ds.GetOptionOrderbookDataInflux(option)
		ot.Type = option.OptionType

		if option.OptionType == dia.CallOption {
			ot.CallAsk = orderbook.AskPrice
			ot.CallBid = orderbook.BidPrice
			strike := fmt.Sprintf("%f", option.StrikePrice)
			nearTermCombined[strike+"-C"] = ot
		}
		if option.OptionType == dia.PutOption {
			ot.PutAsk = orderbook.AskPrice
			ot.PutBid = orderbook.BidPrice

			strike := fmt.Sprintf("%f", option.StrikePrice)
			nearTermCombined[strike+"-P"] = ot
		}
		nearTerm[option.StrikePrice] = ot

	}

	for i, v := range nearTermCombined {
 		if strings.Contains(i, "C") {
			fmt.Println(nearTermCombined[strings.Replace(i, "C", "P", -1)])
		}
		fmt.Printf("name %v, StrikePrice %v , Type %v,  PutAsk %v, PutBid %v, CallAsk %v, CallBid %v \n",i,v.StrikePrice,v.Type,v.PutAsk,v.PutBid,v.CallAsk,v.CallBid)

	}

	nearTerm = CalculateMidAndDifference(nearTerm)
	nextTerm = CalculateMidAndDifference(nextTerm)

	// Minimum Mid is the strike price used to Select  Option which is used for Forward Index calculation
	minimumDiffStrikePriceNearterm := FindMinimumMid(nearTerm)
	log.Infoln("Option Selected for calculating Forward Index for near", nearTerm[minimumDiffStrikePriceNearterm])
	minimumDiffStrikePriceNextterm := FindMinimumMid(nextTerm)
	log.Infoln("Option Selected for calculating Forward Index for next", nextTerm[minimumDiffStrikePriceNextterm])

	f1Data := nearTerm[minimumDiffStrikePriceNearterm]
	f2Data := nextTerm[minimumDiffStrikePriceNextterm]
	roi := asset.InterestRate

	f1 := filters.CalculateForwardIndex(f1Data.StrikePrice, roi, t1, f1Data.CallMid, f1Data.PutMid)
	f2 := filters.CalculateForwardIndex(f2Data.StrikePrice, roi, t2, f2Data.CallMid, f2Data.PutMid)

	log.Infoln("Forward Index F1", f1)
	log.Infoln("Forward Index F2", f2)

	// Get strike Price which is less or equal to the strike price of Forward Index

	sort.Float64s(nextstrikePrices)
	sort.Float64s(nearstrikePrices)

	log.Infoln("Near Strike Prices", nearstrikePrices)
	log.Infoln("Next Strike Prices", nextstrikePrices)


	k01 := findK1(nearstrikePrices, f1)
	log.Infoln("Strike Price k01", k01)

	if k01 == 0.0{
		log.Error("K01 is 0.0 cannot calculate vix")
		return
	}

	k02 := findK2(nextstrikePrices, f2)
	log.Infoln("Strike Price k02", k02)

	if k02 == 0.0{
		log.Error("K02 is 0.0 cannot calculate vix")
		return
	}



	//for _,strikePrice := range nearstrikePrices{
	//	log.Println(nearTerm[strikePrice])
	//}

	/*Select OTM options
	For Near Term
	1) For call option select options whose strike Price is Greater than k0
	2) For Put option select options whose strike Price is Less than k0

	For Near Term
	1) For call option select options whose strike Price is Greater than k1
	2) For Put option select options whose strike Price is Less than k1

	*/

	var (
		nearTermOTM map[float64]OptionsTable
		nextTermOTM map[float64]OptionsTable
	)
	nearTermOTM = make(map[float64]OptionsTable)
	nextTermOTM = make(map[float64]OptionsTable)
	//nextOTMOption := []float64{}

	// OTM for Near Term Call
	nearTermOTM = calculateOTMCall(nearstrikePrices, nearTermCombined, k01)

	nearTermOTMPut := calculateOTMPut(nearstrikePrices, nearTermCombined, k01)
	for k, v := range nearTermOTMPut {
		nearTermOTM[k] = v
	}

	// OTM for Next Term

	nextTermOTM = calculateOTMCall(nextstrikePrices, nextTermCombined, k02)

	nextTermOTMPut := calculateOTMPut(nextstrikePrices, nextTermCombined, k02)

	for k, v := range nextTermOTMPut {
		nextTermOTM[k] = v
	}

	selectedStrikePrice := selectOption(nextTermOTM, nextTerm)
	var (
		nearTermOTMStrikePrices []float64
		nextTermOTMStrikePrices []float64
	)
	for k := range nearTermOTM {
		nearTermOTMStrikePrices = append(nearTermOTMStrikePrices, k)

	}

	log.Infoln("Selected Near", nearTermOTM[selectedStrikePrice])

	for k := range nextTermOTM {
		nextTermOTMStrikePrices = append(nextTermOTMStrikePrices, k)
	}

	selectedStrikePrice = selectOption(nearTermOTM, nextTerm)

	sort.Float64s(nearTermOTMStrikePrices)
	log.Infoln("Total OTM Near Strike Price", len(nearTermOTMStrikePrices))
	sort.Float64s(nextTermOTMStrikePrices)
	log.Infoln("Total OTM Next Strike Price", len(nextTermOTMStrikePrices))

	//Calculate Sigma for ALL OTM

	//previousStrike := 0.0
	nearTermOTM = fillContributionByStrike(nearTermOTM, nearTermOTMStrikePrices, roi, t1, k01)

	//for _, strikePrice := range nearTermOTMStrikePrices {
	//	sigma := CalculateSigma(nearTermOTM[strikePrice], roi, t1)
	//	ot := nearTermOTM[strikePrice]
	//	ot.ContributionByStrike = sigma
	//	nearTermOTM[strikePrice] = ot
	//	//previousStrike = strikePrice
	//}

	//previousStrike = 0.0

	nextTermOTM = fillContributionByStrike(nextTermOTM, nextTermOTMStrikePrices, roi, t2, k02)
	//for _, strikePrice := range nextTermOTMStrikePrices {
	//	sigma := CalculateSigma(nextTermOTM[strikePrice], roi, t2)
	//	ot := nextTermOTM[strikePrice]
	//	ot.ContributionByStrike = sigma
	//	nextTermOTM[strikePrice] = ot
	//	//previousStrike = strikePrice
	//}

	// Sum of sigma

	nearSigma := 0.0
	nextSigma := 0.0

	for _, v := range nearTermOTM {
		nearSigma = nearSigma + v.ContributionByStrike
	}

	for _, v := range nextTermOTM {
		nextSigma = nextSigma + v.ContributionByStrike
	}

	//adjustment https://youtu.be/qToj8UiPBdk?t=613
	nearSigma = nearSigma * (2 / t1)
	nextSigma = nextSigma * (2 / t2)

	log.Infoln("nearSigma", nearSigma)
	log.Infoln("nextSigma", nextSigma)

	//Now calculate σ2 1 and σ2 2:

	cvinear := math.Abs(nearSigma - math.Pow((f1/k01)-1, 2)/t1)

	cvinext := math.Abs(nextSigma - math.Pow((f2/k02)-1, 2)/t2)

	log.Infoln("cvinear", cvinear)
	log.Infoln("cvinext", cvinext)

	vix := 100 * math.Sqrt((t1*cvinear*(46394-43200/46394-35942))+(t2*cvinext*(43200-35924/46394-35924))*525600/43200)

	log.Infoln("Saving CVI", vix)
	err := filters.ETHCVIToDatastore(vix)
	if err != nil {
		log.Error(err)
	}
}

// delta K/ k^2 * e^rt * q()
func CalculateSigmaPut(ot OptionsTable, roi float64, t float64) float64 {
	ans := (ot.DeltaK / math.Pow(ot.StrikePrice, 2)) * math.Exp(roi*t) * math.Abs((ot.PutBid+ot.PutAsk)/2)
	//fmt.Println("CalculateSigma", ot.StrikePrice)
	//fmt.Println("ot.PutBid", ot.PutBid)
	//fmt.Println("ot.PutAsk", ot.PutAsk)
	//fmt.Println("ot.mid", math.Abs((ot.PutBid+ot.PutAsk)/2))
	//fmt.Println("sigma", ans)
	//fmt.Println("Type", ot.Type)
	//fmt.Println("-----------------")
	return ans

}
func CalculateSigmaCall(ot OptionsTable, roi float64, t float64) float64 {
	ans := (ot.DeltaK / math.Pow(ot.StrikePrice, 2)) * math.Exp(roi*t) * math.Abs((ot.CallBid+ot.CallAsk)/2)
	//fmt.Println("CalculateSigma", ot.StrikePrice)
	//fmt.Println("ot.CallBid", ot.CallBid)
	//fmt.Println("ot.CallAsk", ot.CallAsk)
	//fmt.Println("ot.mid", math.Abs((ot.CallBid+ot.CallAsk)/2))
	//fmt.Println("sigma", ans)
	//fmt.Println("Type", ot.Type)
	//fmt.Println("-----------------")
	return ans
}

func CalculateSigmaCallPut(ot OptionsTable, roi float64, t float64) float64 {

	call := math.Abs((ot.CallBid + ot.CallAsk) / 2)
	put := math.Abs((ot.PutBid + ot.PutAsk) / 2)

	ans := (5 / math.Pow(ot.StrikePrice, 2)) * math.Exp(roi*t) * math.Abs((call+put)/2)
	//fmt.Println("CalculateSigmaCallPut", ot.StrikePrice)
	//fmt.Println("ot.PutBid", ot.PutBid)
	//fmt.Println("ot.PutAsk", ot.PutAsk)
	//fmt.Println("ot.CallBid", ot.CallBid)
	//fmt.Println("ot.CallAsk", ot.CallAsk)
	//fmt.Println("ot.mid call", math.Abs((ot.CallBid+ot.CallAsk)/2))
	//
	//fmt.Println("ot.mid put", math.Abs((ot.PutBid+ot.PutAsk)/2))
	//fmt.Println("sigma", ans)
	//fmt.Println("Type", ot.Type)
	//fmt.Println("-----------------")
	return ans
}

func FindMinimumMid(m map[float64]OptionsTable) (minimumStrikePrice float64) {
	var minimumDifference float64
	minimumDifference = 100000000000000000
	for strikePrice, table := range m {
		if minimumDifference > table.Difference {
			minimumStrikePrice = strikePrice
			minimumDifference = table.Difference
		}
	}
	return
}

func CalculateMidAndDifference(m map[float64]OptionsTable) (calculated map[float64]OptionsTable) {

	var key float64
	for key = range m {
		//calculate call and put Mid
		v := m[key]
		v.CallMid = (v.CallAsk + v.CallBid) / 2
		v.PutMid = (v.PutAsk + v.PutBid) / 2
		v.Difference = math.Abs(v.CallMid - v.PutMid)
		v.StrikePrice = key
		m[key] = v
		//if v.Difference == 0 {
		//	delete(m, key)
		//}
	}
	calculated = m
	return

}

func unique(floatSlice []float64) []float64 {
	keys := make(map[float64]bool)
	list := []float64{}
	for _, entry := range floatSlice {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}

func calculateOTMCall(strikePrices []float64, optionTableCombined map[string]OptionsTable, k float64) map[float64]OptionsTable {
	var otm (map[float64]OptionsTable)
	otm = make(map[float64]OptionsTable)
	lastStrikePrice := 0.0

	sort.Sort(sort.Reverse(sort.Float64Slice(strikePrices)))

	fmt.Println("strikePrices", strikePrices)
	for _, strikePrice := range strikePrices {
		if lastStrikePrice != 0.0 {
			// OTM for Call Option
			strikePriceStr := fmt.Sprintf("%f", strikePrice)
			lastStrikePriceStr := fmt.Sprintf("%f", lastStrikePrice)
			optionTableCall := optionTableCombined[strikePriceStr+"-C"]
			lastoptionTableCall := optionTableCombined[lastStrikePriceStr+"-C"]
			//fmt.Printf("-------------optionTableCall %v type %v \n ", optionTableCall.StrikePrice, optionTableCall.Type)
			//fmt.Printf("-------------lastoptionTableCall %v type %v \n ", lastoptionTableCall.CallBid, optionTableCall.CallBid)

			if optionTableCall.Type == dia.CallOption {
				if lastoptionTableCall.CallBid == 0.0 && optionTableCall.CallBid == 0.0 {
					//skipping as last call price in 0.0
					log.Infoln("Skipping as last call Bid is 0.0", strikePrice)
					lastStrikePrice = strikePrice
					//Delete all
					for k := range otm {
						if otm[k].Type == dia.CallOption {
							delete(otm, k)
						}
					}
					continue
				}
				if optionTableCall.CallBid == 0.0 {
					lastStrikePrice = strikePrice

					continue
				}

				if strikePrice > k {
					otm[strikePrice] = optionTableCall
				}
			}

		}
		lastStrikePrice = strikePrice

	}
	return otm

}

func calculateOTMPut(strikePrices []float64, optionTableCombined map[string]OptionsTable, k float64) map[float64]OptionsTable {
	var otm (map[float64]OptionsTable)
	otm = make(map[float64]OptionsTable)
	lastStrikePrice := 0.0
	sort.Float64s(strikePrices)
	for _, strikePrice := range strikePrices {
		if lastStrikePrice != 0.0 {
			// OTM for Call Option
			strikePriceStr := fmt.Sprintf("%f", strikePrice)
			lastStrikePriceStr := fmt.Sprintf("%f", lastStrikePrice)
			optionTablePut := optionTableCombined[strikePriceStr+"-P"]
			lastoptionTablePut := optionTableCombined[lastStrikePriceStr+"-P"]
			if optionTablePut.Type == dia.PutOption {
				if lastoptionTablePut.PutBid == 0.0 && optionTablePut.PutBid == 0.0 {
					//skipping as last call price in 0.0
					log.Infoln("Skipping as last call Put is 0.0", lastoptionTablePut.PutBid)
					lastStrikePrice = strikePrice
					//Delete all
					for k := range otm {
						if otm[k].Type == dia.PutOption {
							delete(otm, k)
						}
					}
					continue
				}
				if optionTablePut.PutBid == 0.0 {
					lastStrikePrice = strikePrice
					continue
				}
				log.Infoln("strikePrice k", strikePrice, k)

				if strikePrice < k {
					otm[strikePrice] = optionTablePut
				}
			}

		}
		lastStrikePrice = strikePrice

	}
	return otm

}

func findK1(nearstrikePrices []float64, forwardLevel float64) float64 {
	k01 := 0.0
	for _, v := range nearstrikePrices {
		if v < forwardLevel && v > k01 {
			k01 = v
		}
	}
	return k01
}

func findK2(nextstrikePrices []float64, forwardLevel float64) float64 {
	k02 := 0.0
	for _, v := range nextstrikePrices {
		if v < forwardLevel && v > k02 {
			k02 = v
		}
	}
	return k02
}

func selectOption(nearTermOTM map[float64]OptionsTable, allOptions map[float64]OptionsTable) float64 {
	maxDiff := 100000000000000000000.0
	selectedStrikePrice := 0.0

	for k := range nearTermOTM {
		fmt.Println("----", allOptions[k].Difference)
		if allOptions[k].Difference < maxDiff {
			maxDiff = allOptions[k].Difference
			selectedStrikePrice = k
		}
	}

	return selectedStrikePrice

}

func fillDeltaK(options map[float64]OptionsTable) map[float64]OptionsTable {
	var sortedStrikePrice []float64
	tableWithDeltaK := make(map[float64]OptionsTable)
	for strikePrice, _ := range options {
		sortedStrikePrice = append(sortedStrikePrice, strikePrice)
	}
	sort.Float64s(sortedStrikePrice)
	for index, strikePrice := range sortedStrikePrice {
		var deltak float64
		if index > 1 && index+1 < len(sortedStrikePrice) {

			deltak = sortedStrikePrice[index+1] - sortedStrikePrice[index-1]
			deltak = deltak / 2
			//fmt.Printf(" value %v last %v next %v  delta %v \n", sortedStrikePrice[index], sortedStrikePrice[index+1], sortedStrikePrice[index-1], deltak)

		}
		// Last
		if index+1 == len(sortedStrikePrice) {
			deltak = sortedStrikePrice[index] - sortedStrikePrice[index-1]
			deltak = deltak
			//fmt.Printf(" value %v last %v next %v  delta %v \n", sortedStrikePrice[index], sortedStrikePrice[index], sortedStrikePrice[index-1], deltak)

		}
		//first
		if index == 0 {
			deltak = sortedStrikePrice[index+1] - sortedStrikePrice[index]
			deltak = deltak
			//fmt.Printf("value %v last %v next %v  delta %v \n", sortedStrikePrice[index], sortedStrikePrice[index+1], sortedStrikePrice[index], deltak)

		}

		if index == 1 {
			deltak = sortedStrikePrice[index+1] - sortedStrikePrice[index]
			deltak = deltak
			//fmt.Printf("value %v last %v next %v  delta %v \n", sortedStrikePrice[index], sortedStrikePrice[index+1], sortedStrikePrice[index], deltak)

		}

		ot := options[strikePrice]
		ot.DeltaK = deltak
		tableWithDeltaK[strikePrice] = ot
	}

	return tableWithDeltaK

}

func fillContributionByStrike(options map[float64]OptionsTable, strikePrices []float64, roi, t, k float64) map[float64]OptionsTable {
	for strikePrice, ot := range options {
		var sigma float64
		if k > strikePrice {
			sigma = CalculateSigmaPut(options[strikePrice], roi, t)
		} else if k == strikePrice {
			sigma = CalculateSigmaCallPut(options[strikePrice], roi, t)
		} else {
			sigma = CalculateSigmaCall(options[strikePrice], roi, t)
		}
		ot.ContributionByStrike = sigma
		options[strikePrice] = ot
		//previousStrike = strikePrice
	}
	return options

}

func PutCallK(option OptionsTable) {

}
