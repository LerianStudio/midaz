/*!40101 SET @OLD_CHARACTER_SET_CLIENT=@@CHARACTER_SET_CLIENT */;
/*!40101 SET @OLD_CHARACTER_SET_RESULTS=@@CHARACTER_SET_RESULTS */;
/*!40101 SET @OLD_COLLATION_CONNECTION=@@COLLATION_CONNECTION */;
/*!40101 SET NAMES utf8mb4 */;
/*!40103 SET @OLD_TIME_ZONE=@@TIME_ZONE */;
/*!40103 SET TIME_ZONE='+00:00' */;
/*!40014 SET @OLD_UNIQUE_CHECKS=@@UNIQUE_CHECKS, UNIQUE_CHECKS=0 */;
/*!40014 SET @OLD_FOREIGN_KEY_CHECKS=@@FOREIGN_KEY_CHECKS, FOREIGN_KEY_CHECKS=0 */;
/*!40101 SET @OLD_SQL_MODE=@@SQL_MODE, SQL_MODE='NO_AUTO_VALUE_ON_ZERO' */;
/*!40111 SET @OLD_SQL_NOTES=@@SQL_NOTES, SQL_NOTES=0 */;

--
-- Table structure for table `LeafData`
--

DROP TABLE IF EXISTS `LeafData`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `LeafData` (
                            `TreeId` bigint(20) NOT NULL,
                            `LeafIdentityHash` varbinary(255) NOT NULL,
                            `LeafValue` longblob NOT NULL,
                            `ExtraData` longblob DEFAULT NULL,
                            `QueueTimestampNanos` bigint(20) NOT NULL,
                            PRIMARY KEY (`TreeId`,`LeafIdentityHash`),
                            CONSTRAINT `LeafData_ibfk_1` FOREIGN KEY (`TreeId`) REFERENCES `Trees` (`TreeId`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=latin1;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `LeafData`
--

LOCK TABLES `LeafData` WRITE;
/*!40000 ALTER TABLE `LeafData` DISABLE KEYS */;
/*!40000 ALTER TABLE `LeafData` ENABLE KEYS */;
UNLOCK TABLES;

--
-- Table structure for table `MapHead`
--

DROP TABLE IF EXISTS `MapHead`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `MapHead` (
                           `TreeId` bigint(20) NOT NULL,
                           `MapHeadTimestamp` bigint(20) NOT NULL,
                           `RootHash` varbinary(255) NOT NULL,
                           `MapRevision` bigint(20) DEFAULT NULL,
                           `RootSignature` varbinary(1024) NOT NULL,
                           `MapperData` mediumblob DEFAULT NULL,
                           PRIMARY KEY (`TreeId`,`MapHeadTimestamp`),
                           UNIQUE KEY `MapHeadRevisionIdx` (`TreeId`,`MapRevision`),
                           CONSTRAINT `MapHead_ibfk_1` FOREIGN KEY (`TreeId`) REFERENCES `Trees` (`TreeId`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=latin1;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `MapHead`
--

LOCK TABLES `MapHead` WRITE;
/*!40000 ALTER TABLE `MapHead` DISABLE KEYS */;
/*!40000 ALTER TABLE `MapHead` ENABLE KEYS */;
UNLOCK TABLES;

--
-- Table structure for table `MapLeaf`
--

DROP TABLE IF EXISTS `MapLeaf`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `MapLeaf` (
                           `TreeId` bigint(20) NOT NULL,
                           `KeyHash` varbinary(255) NOT NULL,
                           `MapRevision` bigint(20) NOT NULL,
                           `LeafValue` longblob NOT NULL,
                           PRIMARY KEY (`TreeId`,`KeyHash`,`MapRevision`),
                           CONSTRAINT `MapLeaf_ibfk_1` FOREIGN KEY (`TreeId`) REFERENCES `Trees` (`TreeId`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=latin1;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `MapLeaf`
--

LOCK TABLES `MapLeaf` WRITE;
/*!40000 ALTER TABLE `MapLeaf` DISABLE KEYS */;
/*!40000 ALTER TABLE `MapLeaf` ENABLE KEYS */;
UNLOCK TABLES;

--
-- Table structure for table `SequencedLeafData`
--

DROP TABLE IF EXISTS `SequencedLeafData`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `SequencedLeafData` (
                                     `TreeId` bigint(20) NOT NULL,
                                     `SequenceNumber` bigint(20) unsigned NOT NULL,
                                     `LeafIdentityHash` varbinary(255) NOT NULL,
                                     `MerkleLeafHash` varbinary(255) NOT NULL,
                                     `IntegrateTimestampNanos` bigint(20) NOT NULL,
                                     PRIMARY KEY (`TreeId`,`SequenceNumber`),
                                     KEY `TreeId` (`TreeId`,`LeafIdentityHash`),
                                     KEY `SequencedLeafMerkleIdx` (`TreeId`,`MerkleLeafHash`),
                                     CONSTRAINT `SequencedLeafData_ibfk_1` FOREIGN KEY (`TreeId`) REFERENCES `Trees` (`TreeId`) ON DELETE CASCADE,
                                     CONSTRAINT `SequencedLeafData_ibfk_2` FOREIGN KEY (`TreeId`, `LeafIdentityHash`) REFERENCES `LeafData` (`TreeId`, `LeafIdentityHash`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=latin1;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `SequencedLeafData`
--

LOCK TABLES `SequencedLeafData` WRITE;
/*!40000 ALTER TABLE `SequencedLeafData` DISABLE KEYS */;
/*!40000 ALTER TABLE `SequencedLeafData` ENABLE KEYS */;
UNLOCK TABLES;

--
-- Table structure for table `Subtree`
--

DROP TABLE IF EXISTS `Subtree`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `Subtree` (
                           `TreeId` bigint(20) NOT NULL,
                           `SubtreeId` varbinary(255) NOT NULL,
                           `Nodes` mediumblob NOT NULL,
                           `SubtreeRevision` int(11) NOT NULL,
                           PRIMARY KEY (`TreeId`,`SubtreeId`,`SubtreeRevision`),
                           CONSTRAINT `Subtree_ibfk_1` FOREIGN KEY (`TreeId`) REFERENCES `Trees` (`TreeId`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=latin1;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `Subtree`
--

LOCK TABLES `Subtree` WRITE;
/*!40000 ALTER TABLE `Subtree` DISABLE KEYS */;
/*!40000 ALTER TABLE `Subtree` ENABLE KEYS */;
UNLOCK TABLES;

--
-- Table structure for table `TreeControl`
--

DROP TABLE IF EXISTS `TreeControl`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `TreeControl` (
                               `TreeId` bigint(20) NOT NULL,
                               `SigningEnabled` tinyint(1) NOT NULL,
                               `SequencingEnabled` tinyint(1) NOT NULL,
                               `SequenceIntervalSeconds` int(11) NOT NULL,
                               PRIMARY KEY (`TreeId`),
                               CONSTRAINT `TreeControl_ibfk_1` FOREIGN KEY (`TreeId`) REFERENCES `Trees` (`TreeId`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=latin1;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `TreeControl`
--

LOCK TABLES `TreeControl` WRITE;
/*!40000 ALTER TABLE `TreeControl` DISABLE KEYS */;
/*!40000 ALTER TABLE `TreeControl` ENABLE KEYS */;
UNLOCK TABLES;

--
-- Table structure for table `TreeHead`
--

DROP TABLE IF EXISTS `TreeHead`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `TreeHead` (
                            `TreeId` bigint(20) NOT NULL,
                            `TreeHeadTimestamp` bigint(20) NOT NULL,
                            `TreeSize` bigint(20) DEFAULT NULL,
                            `RootHash` varbinary(255) NOT NULL,
                            `RootSignature` varbinary(1024) NOT NULL,
                            `TreeRevision` bigint(20) DEFAULT NULL,
                            PRIMARY KEY (`TreeId`,`TreeHeadTimestamp`),
                            UNIQUE KEY `TreeHeadRevisionIdx` (`TreeId`,`TreeRevision`),
                            CONSTRAINT `TreeHead_ibfk_1` FOREIGN KEY (`TreeId`) REFERENCES `Trees` (`TreeId`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=latin1;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `TreeHead`
--

LOCK TABLES `TreeHead` WRITE;
/*!40000 ALTER TABLE `TreeHead` DISABLE KEYS */;
/*!40000 ALTER TABLE `TreeHead` ENABLE KEYS */;
UNLOCK TABLES;

--
-- Table structure for table `Trees`
--

DROP TABLE IF EXISTS `Trees`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `Trees` (
                         `TreeId` bigint(20) NOT NULL,
                         `TreeState` enum('ACTIVE','FROZEN','DRAINING') NOT NULL,
                         `TreeType` enum('LOG','MAP','PREORDERED_LOG') NOT NULL,
                         `HashStrategy` enum('RFC6962_SHA256','TEST_MAP_HASHER','OBJECT_RFC6962_SHA256','CONIKS_SHA512_256','CONIKS_SHA256') NOT NULL,
                         `HashAlgorithm` enum('SHA256') NOT NULL,
                         `SignatureAlgorithm` enum('ECDSA','RSA') NOT NULL,
                         `DisplayName` varchar(20) DEFAULT NULL,
                         `Description` varchar(200) DEFAULT NULL,
                         `CreateTimeMillis` bigint(20) NOT NULL,
                         `UpdateTimeMillis` bigint(20) NOT NULL,
                         `MaxRootDurationMillis` bigint(20) NOT NULL,
                         `PrivateKey` mediumblob NOT NULL,
                         `PublicKey` mediumblob NOT NULL,
                         `Deleted` tinyint(1) DEFAULT NULL,
                         `DeleteTimeMillis` bigint(20) DEFAULT NULL,
                         PRIMARY KEY (`TreeId`)
) ENGINE=InnoDB DEFAULT CHARSET=latin1;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `Trees`
--

LOCK TABLES `Trees` WRITE;
/*!40000 ALTER TABLE `Trees` DISABLE KEYS */;
/*!40000 ALTER TABLE `Trees` ENABLE KEYS */;
UNLOCK TABLES;

--
-- Table structure for table `Unsequenced`
--

DROP TABLE IF EXISTS `Unsequenced`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `Unsequenced` (
                               `TreeId` bigint(20) NOT NULL,
                               `Bucket` int(11) NOT NULL,
                               `LeafIdentityHash` varbinary(255) NOT NULL,
                               `MerkleLeafHash` varbinary(255) NOT NULL,
                               `QueueTimestampNanos` bigint(20) NOT NULL,
                               `QueueID` varbinary(32) DEFAULT NULL,
                               PRIMARY KEY (`TreeId`,`Bucket`,`QueueTimestampNanos`,`LeafIdentityHash`),
                               UNIQUE KEY `QueueID` (`QueueID`)
) ENGINE=InnoDB DEFAULT CHARSET=latin1;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `Unsequenced`
--

LOCK TABLES `Unsequenced` WRITE;
/*!40000 ALTER TABLE `Unsequenced` DISABLE KEYS */;
/*!40000 ALTER TABLE `Unsequenced` ENABLE KEYS */;
UNLOCK TABLES;
/*!40103 SET TIME_ZONE=@OLD_TIME_ZONE */;

/*!40101 SET SQL_MODE=@OLD_SQL_MODE */;
/*!40014 SET FOREIGN_KEY_CHECKS=@OLD_FOREIGN_KEY_CHECKS */;
/*!40014 SET UNIQUE_CHECKS=@OLD_UNIQUE_CHECKS */;
/*!40101 SET CHARACTER_SET_CLIENT=@OLD_CHARACTER_SET_CLIENT */;
/*!40101 SET CHARACTER_SET_RESULTS=@OLD_CHARACTER_SET_RESULTS */;
/*!40101 SET COLLATION_CONNECTION=@OLD_COLLATION_CONNECTION */;
/*!40111 SET SQL_NOTES=@OLD_SQL_NOTES */;