(() => {
	return {
 		"title": document.querySelector('.block-detail .title')?.textContent?.trim(),
 		"description": document.getElementById('torrentsdesc')?.textContent?.trim(),
	};
})()
