CREATE TABLE IF NOT EXISTS `unlockr_users` (
  `id` int NOT NULL AUTO_INCREMENT,
  `username` varchar(30) NOT NULL,
  `nickname` varchar(30) NOT NULL,
  `password_hash` varchar(255) NOT NULL,

  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `unlockr_group_memberships` (
  `id` int NOT NULL AUTO_INCREMENT,
  `username` varchar(30) NOT NULL,
  `group_name` varchar(30) NOT NULL,

  PRIMARY KEY (`id`),
  FOREIGN KEY (`username`)
    REFERENCES `unlockr_users` (`username`)
    ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `unlockr_sessions` (
  `id` varchar(50) NOT NULL,
  `username` varchar(30) NOT NULL,
  `expiry` BIGINT UNSIGNED NOT NULL,
  `extra` JSON,

  PRIMARY KEY (`id`),
  UNIQUE KEY (`id`),
  KEY (`username`),
  FOREIGN KEY (`username`)
    REFERENCES `unlockr_users` (`username`)
    ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
