export namespace main {
	
	export class BusinessResult {
	    id: number;
	    job_id: string;
	    job_name: string;
	    location: string;
	    keywords: string;
	    map_url: string;
	    shop_name: string;
	    category: string;
	    address: string;
	    open_hours: string;
	    phone: string;
	    review_count: string;
	    rating: string;
	    latitude: string;
	    longitude: string;
	    email: string;
	    website: string;
	    imported: boolean;
	    created_at: string;
	
	    static createFrom(source: any = {}) {
	        return new BusinessResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.job_id = source["job_id"];
	        this.job_name = source["job_name"];
	        this.location = source["location"];
	        this.keywords = source["keywords"];
	        this.map_url = source["map_url"];
	        this.shop_name = source["shop_name"];
	        this.category = source["category"];
	        this.address = source["address"];
	        this.open_hours = source["open_hours"];
	        this.phone = source["phone"];
	        this.review_count = source["review_count"];
	        this.rating = source["rating"];
	        this.latitude = source["latitude"];
	        this.longitude = source["longitude"];
	        this.email = source["email"];
	        this.website = source["website"];
	        this.imported = source["imported"];
	        this.created_at = source["created_at"];
	    }
	}
	export class ResultFilter {
	    job_id: string;
	    job_ids: string[];
	    query: string;
	    category: string;
	    country: string;
	    city: string;
	    location: string;
	    has_phone: boolean;
	    has_email: boolean;
	    has_website: boolean;
	    not_imported: boolean;
	    limit: number;
	
	    static createFrom(source: any = {}) {
	        return new ResultFilter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.job_id = source["job_id"];
	        this.job_ids = source["job_ids"];
	        this.query = source["query"];
	        this.category = source["category"];
	        this.country = source["country"];
	        this.city = source["city"];
	        this.location = source["location"];
	        this.has_phone = source["has_phone"];
	        this.has_email = source["has_email"];
	        this.has_website = source["has_website"];
	        this.not_imported = source["not_imported"];
	        this.limit = source["limit"];
	    }
	}
	export class ScraperStartRequest {
	    mode: string;
	    country: string;
	    location: string;
	    keywords: string[];
	
	    static createFrom(source: any = {}) {
	        return new ScraperStartRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.country = source["country"];
	        this.location = source["location"];
	        this.keywords = source["keywords"];
	    }
	}

}

export namespace whatsapp {
	
	export class Message {
	    text: string;
	    image_id: string;
	    pdf_id: string;
	
	    static createFrom(source: any = {}) {
	        return new Message(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	        this.image_id = source["image_id"];
	        this.pdf_id = source["pdf_id"];
	    }
	}
	export class SendOptions {
	    contact_delay_min_seconds: number;
	    contact_delay_max_seconds: number;
	    batch_size: number;
	    batch_delay_min_seconds: number;
	    batch_delay_max_seconds: number;
	    max_consecutive_failures: number;
	
	    static createFrom(source: any = {}) {
	        return new SendOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.contact_delay_min_seconds = source["contact_delay_min_seconds"];
	        this.contact_delay_max_seconds = source["contact_delay_max_seconds"];
	        this.batch_size = source["batch_size"];
	        this.batch_delay_min_seconds = source["batch_delay_min_seconds"];
	        this.batch_delay_max_seconds = source["batch_delay_max_seconds"];
	        this.max_consecutive_failures = source["max_consecutive_failures"];
	    }
	}

}

