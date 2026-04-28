package geodata

import (
	"context"
	"math"
	"strings"

	"github.com/gosom/google-maps-scraper/grid"
)

type Area struct {
	Name   string
	BBox   grid.BoundingBox
	CellKm float64
}

type Country struct {
	DisplayName string
	CountryCode string
	BBox        grid.BoundingBox
	Areas       []Area
}

func ResolveCountry(location string) (Country, bool) {
	key := strings.ToLower(strings.TrimSpace(location))

	switch key {
	case "英国", "uk", "united kingdom", "great britain":
		return unitedKingdom(), true
	case "日本", "japan":
		return japan(), true
	default:
		return Country{}, false
	}
}

func unitedKingdom() Country {
	countryBBox := grid.BoundingBox{
		MinLat: 49.6740,
		MinLon: -8.6500,
		MaxLat: 61.0610,
		MaxLon: 1.7680,
	}

	areas := []Area{
		area("London, United Kingdom", 51.5074, -0.1278, 28, 3),
		area("Birmingham, United Kingdom", 52.4862, -1.8904, 16, 3),
		area("Manchester, United Kingdom", 53.4808, -2.2426, 16, 3),
		area("Glasgow, United Kingdom", 55.8642, -4.2518, 16, 3),
		area("Liverpool, United Kingdom", 53.4084, -2.9916, 14, 3),
		area("Leeds, United Kingdom", 53.8008, -1.5491, 14, 3),
		area("Sheffield, United Kingdom", 53.3811, -1.4701, 12, 3),
		area("Bristol, United Kingdom", 51.4545, -2.5879, 12, 3),
		area("Edinburgh, United Kingdom", 55.9533, -3.1883, 12, 3),
		area("Cardiff, United Kingdom", 51.4816, -3.1791, 12, 3),
		area("Leicester, United Kingdom", 52.6369, -1.1398, 10, 3),
		area("Coventry, United Kingdom", 52.4068, -1.5197, 10, 3),
		area("Nottingham, United Kingdom", 52.9548, -1.1581, 10, 3),
		area("Newcastle upon Tyne, United Kingdom", 54.9783, -1.6178, 10, 3),
		area("Sunderland, United Kingdom", 54.9069, -1.3838, 9, 3),
		area("Brighton, United Kingdom", 50.8225, -0.1372, 9, 3),
		area("Hull, United Kingdom", 53.7676, -0.3274, 9, 3),
		area("Plymouth, United Kingdom", 50.3755, -4.1427, 9, 3),
		area("Stoke-on-Trent, United Kingdom", 53.0027, -2.1794, 9, 3),
		area("Wolverhampton, United Kingdom", 52.5862, -2.1288, 9, 3),
		area("Derby, United Kingdom", 52.9225, -1.4746, 9, 3),
		area("Swansea, United Kingdom", 51.6214, -3.9436, 9, 3),
		area("Southampton, United Kingdom", 50.9097, -1.4044, 9, 3),
		area("Portsmouth, United Kingdom", 50.8198, -1.0880, 9, 3),
		area("York, United Kingdom", 53.9590, -1.0815, 8, 3),
		area("Oxford, United Kingdom", 51.7520, -1.2577, 8, 3),
		area("Cambridge, United Kingdom", 52.2053, 0.1218, 8, 3),
		area("Reading, United Kingdom", 51.4543, -0.9781, 8, 3),
		area("Luton, United Kingdom", 51.8787, -0.4200, 8, 3),
		area("Milton Keynes, United Kingdom", 52.0406, -0.7594, 8, 3),
		area("Northampton, United Kingdom", 52.2405, -0.9027, 8, 3),
		area("Norwich, United Kingdom", 52.6309, 1.2974, 8, 3),
		area("Ipswich, United Kingdom", 52.0567, 1.1482, 8, 3),
		area("Peterborough, United Kingdom", 52.5695, -0.2405, 8, 3),
		area("Middlesbrough, United Kingdom", 54.5742, -1.2348, 8, 3),
		area("Blackpool, United Kingdom", 53.8175, -3.0357, 8, 3),
		area("Preston, United Kingdom", 53.7632, -2.7031, 8, 3),
		area("Bolton, United Kingdom", 53.5769, -2.4282, 8, 3),
		area("Wigan, United Kingdom", 53.5451, -2.6325, 8, 3),
		area("Bournemouth, United Kingdom", 50.7192, -1.8808, 8, 3),
		area("Swindon, United Kingdom", 51.5558, -1.7797, 8, 3),
		area("Exeter, United Kingdom", 50.7184, -3.5339, 8, 3),
		area("Gloucester, United Kingdom", 51.8642, -2.2382, 8, 3),
		area("Aberdeen, United Kingdom", 57.1497, -2.0943, 9, 3),
		area("Dundee, United Kingdom", 56.4620, -2.9707, 8, 3),
		area("Inverness, United Kingdom", 57.4778, -4.2247, 8, 3),
		area("Belfast, United Kingdom", 54.5973, -5.9301, 12, 3),
		area("Derry, United Kingdom", 54.9966, -7.3086, 8, 3),
		area("Newport, United Kingdom", 51.5842, -2.9977, 8, 3),
		area("Wrexham, United Kingdom", 53.0465, -2.9916, 8, 3),
	}

	return Country{
		DisplayName: "United Kingdom",
		CountryCode: "GB",
		BBox:        countryBBox,
		Areas:       areas,
	}
}

func japan() Country {
	countryBBox := japanBoundingBox()

	areas := []Area{
		area("Tokyo, Japan", 35.6762, 139.6503, 35, 3),
		area("Yokohama, Japan", 35.4437, 139.6380, 18, 3),
		area("Osaka, Japan", 34.6937, 135.5023, 22, 3),
		area("Nagoya, Japan", 35.1815, 136.9066, 18, 3),
		area("Sapporo, Japan", 43.0618, 141.3545, 18, 3),
		area("Fukuoka, Japan", 33.5902, 130.4017, 16, 3),
		area("Kobe, Japan", 34.6901, 135.1955, 14, 3),
		area("Kyoto, Japan", 35.0116, 135.7681, 14, 3),
		area("Kawasaki, Japan", 35.5308, 139.7036, 12, 3),
		area("Saitama, Japan", 35.8617, 139.6455, 14, 3),
		area("Hiroshima, Japan", 34.3853, 132.4553, 14, 3),
		area("Sendai, Japan", 38.2682, 140.8694, 14, 3),
		area("Chiba, Japan", 35.6074, 140.1065, 14, 3),
		area("Kitakyushu, Japan", 33.8834, 130.8751, 14, 3),
		area("Sakai, Japan", 34.5733, 135.4828, 12, 3),
		area("Niigata, Japan", 37.9161, 139.0364, 12, 3),
		area("Hamamatsu, Japan", 34.7108, 137.7261, 12, 3),
		area("Kumamoto, Japan", 32.8031, 130.7079, 12, 3),
		area("Sagamihara, Japan", 35.5714, 139.3733, 12, 3),
		area("Okayama, Japan", 34.6551, 133.9195, 12, 3),
		area("Shizuoka, Japan", 34.9756, 138.3828, 12, 3),
		area("Kagoshima, Japan", 31.5966, 130.5571, 12, 3),
		area("Funabashi, Japan", 35.6947, 139.9826, 10, 3),
		area("Hachioji, Japan", 35.6558, 139.3239, 10, 3),
		area("Himeji, Japan", 34.8151, 134.6853, 10, 3),
		area("Matsuyama, Japan", 33.8392, 132.7657, 10, 3),
		area("Utsunomiya, Japan", 36.5551, 139.8828, 10, 3),
		area("Matsudo, Japan", 35.7876, 139.9032, 10, 3),
		area("Nishinomiya, Japan", 34.7376, 135.3416, 10, 3),
		area("Kurashiki, Japan", 34.5850, 133.7722, 10, 3),
		area("Oita, Japan", 33.2396, 131.6093, 10, 3),
		area("Kanazawa, Japan", 36.5613, 136.6562, 10, 3),
		area("Fukuyama, Japan", 34.4859, 133.3623, 10, 3),
		area("Toyama, Japan", 36.6953, 137.2113, 10, 3),
		area("Nagasaki, Japan", 32.7503, 129.8777, 10, 3),
		area("Nara, Japan", 34.6851, 135.8048, 10, 3),
		area("Nagano, Japan", 36.6486, 138.1948, 10, 3),
		area("Gifu, Japan", 35.4233, 136.7607, 10, 3),
		area("Miyazaki, Japan", 31.9077, 131.4202, 10, 3),
		area("Naha, Japan", 26.2124, 127.6792, 10, 3),
	}

	return Country{
		DisplayName: "Japan",
		CountryCode: "JP",
		BBox:        countryBBox,
		Areas:       areas,
	}
}

func CountryFromCityStore(
	ctx context.Context,
	store *CityStore,
	countryCode string,
	displayName string,
	bbox grid.BoundingBox,
	limit int,
) (Country, error) {
	cities, err := store.TopCities(ctx, countryCode, limit)
	if err != nil {
		return Country{}, err
	}

	areas := make([]Area, 0, len(cities))
	for _, city := range cities {
		areas = append(areas, Area{
			Name:   city.ASCIIName + ", " + displayName,
			BBox:   bboxAround(city.Latitude, city.Longitude, radiusForPopulation(city.Population)),
			CellKm: cellSizeForPopulation(city.Population),
		})
	}

	return Country{
		DisplayName: displayName,
		CountryCode: strings.ToUpper(countryCode),
		BBox:        bbox,
		Areas:       areas,
	}, nil
}

func japanBoundingBox() grid.BoundingBox {
	return grid.BoundingBox{
		MinLat: 24.0,
		MinLon: 122.0,
		MaxLat: 46.0,
		MaxLon: 146.0,
	}
}

func area(name string, lat, lon, radiusKm, cellKm float64) Area {
	return Area{
		Name:   name,
		BBox:   bboxAround(lat, lon, radiusKm),
		CellKm: cellKm,
	}
}

func radiusForPopulation(population int64) float64 {
	switch {
	case population >= 5_000_000:
		return 35
	case population >= 1_000_000:
		return 25
	case population >= 500_000:
		return 18
	case population >= 200_000:
		return 14
	default:
		return 10
	}
}

func cellSizeForPopulation(population int64) float64 {
	if population >= 1_000_000 {
		return 3
	}

	return 4
}

func bboxAround(lat, lon, radiusKm float64) grid.BoundingBox {
	const kmPerDegreeLat = 111.32

	latDelta := radiusKm / kmPerDegreeLat
	cosLat := math.Cos(lat * math.Pi / 180)
	if math.Abs(cosLat) < 1e-6 {
		cosLat = 1e-6
	}
	lonDelta := radiusKm / (kmPerDegreeLat * cosLat)

	return grid.BoundingBox{
		MinLat: lat - latDelta,
		MinLon: lon - lonDelta,
		MaxLat: lat + latDelta,
		MaxLon: lon + lonDelta,
	}
}
